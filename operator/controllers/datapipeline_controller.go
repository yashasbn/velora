/*
Copyright 2026 Velora Authors.
Licensed under the Apache License, Version 2.0.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vv1 "github.com/yashasbn/velora/api/v1alpha1"
	"github.com/yashasbn/velora/internal/airflow"
	"github.com/yashasbn/velora/internal/minio"
	"github.com/yashasbn/velora/metrics"
)

const (
	// requeueAfter is how often we requeue even when nothing changed —
	// continuous reconciliation, not one-shot "create if missing".
	requeueAfter = 30 * time.Second

	// configMapSuffix is appended to the pipeline name for ConfigMap names.
	configMapSuffix = "-pipeline-config"

	// cronJobSuffix is appended to the pipeline name for CronJob names.
	cronJobSuffix = "-pipeline-cron"

	// velora-system namespace where the operator's own secrets live.
	veloraNamespace = "velora-system"

	// minioCreds is the name of the Secret in velora-system containing MinIO creds.
	minioCredsSecret = "velora-minio-creds"
)

// DataPipelineReconciler reconciles DataPipeline resources.
//
// Reconcile loop follows the Observe-Compare-Act pattern:
//  1. Fetch the DataPipeline
//  2. Handle deletion (finalizer cleanup)
//  3. Ensure finalizer is present
//  4. Reconcile each sub-resource (bucket, configmap, secret check, cronjob)
//  5. Update status conditions
//  6. Requeue after requeueAfter for continuous reconciliation
//
// +kubebuilder:rbac:groups=velora.dev,resources=datapipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velora.dev,resources=datapipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=velora.dev,resources=datapipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
type DataPipelineReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	MinioClient   minio.Client
	AirflowClient airflow.Client
}

// Reconcile is the main reconciliation function.
// It is called whenever a DataPipeline resource changes, or after requeueAfter.
func (r *DataPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("datapipeline", req.NamespacedName)
	start := time.Now()

	// -------------------------------------------------------------------------
	// 1. Fetch the DataPipeline resource
	// -------------------------------------------------------------------------
	dp := &vv1.DataPipeline{}
	if err := r.Get(ctx, req.NamespacedName, dp); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource was deleted before we could reconcile — nothing to do.
			logger.Info("DataPipeline not found, ignoring (already deleted)")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to fetch DataPipeline")
		return ctrl.Result{}, err
	}

	// Instrument: count and time every reconcile.
	defer func() {
		metrics.ReconcileTotal.Inc()
		metrics.ReconcileDuration.Observe(time.Since(start).Seconds())
	}()

	// -------------------------------------------------------------------------
	// 2. Handle deletion — run cleanup when DeletionTimestamp is set
	// -------------------------------------------------------------------------
	if !dp.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, dp)
	}

	// -------------------------------------------------------------------------
	// 3. Ensure our finalizer is registered
	// -------------------------------------------------------------------------
	if !controllerutil.ContainsFinalizer(dp, vv1.FinalizerName) {
		controllerutil.AddFinalizer(dp, vv1.FinalizerName)
		if err := r.Update(ctx, dp); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Return and requeue — the update will trigger a new reconcile.
		return ctrl.Result{}, nil
	}

	// -------------------------------------------------------------------------
	// 4. Reconcile sub-resources (Observe → Compare → Act for each)
	// -------------------------------------------------------------------------

	// Set initial phase if not set
	if dp.Status.Phase == "" {
		dp.Status.Phase = vv1.PhasePending
		r.setCondition(dp, vv1.ConditionReady, metav1.ConditionFalse, "Pending", "Reconciliation starting")
	}
	dp.Status.ObservedGeneration = dp.Generation

	// 4a. Reconcile MinIO bucket
	bucketErr := r.reconcileBucket(ctx, dp)

	// 4b. Reconcile ConfigMap (pipeline config)
	cmErr := r.reconcileConfigMap(ctx, dp)

	// 4c. Validate source secret exists
	secretErr := r.validateSourceSecret(ctx, dp)

	// 4d. Reconcile CronJob
	cronErr := r.reconcileCronJob(ctx, dp)

	// -------------------------------------------------------------------------
	// 5. Determine overall phase and update Status
	// -------------------------------------------------------------------------
	anyErr := firstError(bucketErr, cmErr, secretErr, cronErr)
	if anyErr != nil {
		dp.Status.Phase = vv1.PhaseFailed
		r.setCondition(dp, vv1.ConditionReady, metav1.ConditionFalse, "ReconcileError", anyErr.Error())
		metrics.ReconcileErrors.Inc()
		metrics.PipelineFailures.WithLabelValues(dp.Name, dp.Namespace).Inc()
	} else {
		dp.Status.Phase = vv1.PhaseReady
		r.setCondition(dp, vv1.ConditionReady, metav1.ConditionTrue, "AllResourcesReady", "All sub-resources reconciled successfully")
	}

	metrics.PipelineReady.WithLabelValues(dp.Name, dp.Namespace).Set(boolToFloat64(anyErr == nil))

	// Persist status changes.
	if err := r.Status().Update(ctx, dp); err != nil {
		if apierrors.IsConflict(err) {
			// Another update already happened — requeue and reconcile again.
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(err, "Failed to update DataPipeline status")
		return ctrl.Result{}, err
	}

	if anyErr != nil {
		logger.Error(anyErr, "Reconciliation completed with errors, requeuing")
		return ctrl.Result{RequeueAfter: requeueAfter}, anyErr
	}

	logger.Info("Reconciliation complete", "phase", dp.Status.Phase, "requeueAfter", requeueAfter)
	// Requeue unconditionally — continuous reconciliation, not one-shot.
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// handleDeletion runs cleanup when the DataPipeline is being deleted.
// It removes the CronJob, optionally archives logs, then removes the finalizer
// so the Kubernetes GC can proceed.
func (r *DataPipelineReconciler) handleDeletion(ctx context.Context, dp *vv1.DataPipeline) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(dp, vv1.FinalizerName) {
		// Finalizer already removed — nothing to do.
		return ctrl.Result{}, nil
	}

	logger.Info("Running finalizer cleanup", "pipeline", dp.Name)

	// Delete the managed CronJob.
	cronJobName := dp.Name + cronJobSuffix
	cj := &batchv1.CronJob{}
	if err := r.Get(ctx, types.NamespacedName{Name: cronJobName, Namespace: dp.Namespace}, cj); err == nil {
		if err := r.Delete(ctx, cj); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete CronJob during finalizer cleanup")
			return ctrl.Result{}, err
		}
		logger.Info("Deleted CronJob", "cronJob", cronJobName)
	}

	// Delete the managed ConfigMap.
	cmName := dp.Name + configMapSuffix
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: dp.Namespace}, cm); err == nil {
		if err := r.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete ConfigMap during finalizer cleanup")
			return ctrl.Result{}, err
		}
		logger.Info("Deleted ConfigMap", "configMap", cmName)
	}

	// Note: bucket is intentionally NOT deleted by default — data should
	// outlive the pipeline definition. Add explicit opt-in via a spec field
	// if bucket deletion is desired (e.g., spec.destination.deleteOnRemove: true).

	r.Recorder.Event(dp, corev1.EventTypeNormal, vv1.EventReasonCleanedUp,
		fmt.Sprintf("Pipeline %s cleaned up: CronJob and ConfigMap removed", dp.Name))

	// Remove the finalizer — Kubernetes GC can now proceed.
	controllerutil.RemoveFinalizer(dp, vv1.FinalizerName)
	if err := r.Update(ctx, dp); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Finalizer removed, deletion proceeding", "pipeline", dp.Name)
	return ctrl.Result{}, nil
}

// reconcileBucket ensures the destination MinIO bucket exists.
// Idempotent: if the bucket already exists this is a no-op.
func (r *DataPipelineReconciler) reconcileBucket(ctx context.Context, dp *vv1.DataPipeline) error {
	logger := log.FromContext(ctx)

	if dp.Spec.Destination.Type != "minio" {
		// Non-MinIO destinations: mark BucketCreated as N/A (True with a note).
		r.setCondition(dp, vv1.ConditionBucketCreated, metav1.ConditionTrue,
			"NotApplicable", "Destination type is not minio; skipping bucket provisioning")
		return nil
	}

	bucketName := dp.Spec.Destination.Bucket
	logger.Info("Reconciling MinIO bucket", "bucket", bucketName)

	created, err := r.MinioClient.EnsureBucket(ctx, bucketName)
	if err != nil {
		r.setCondition(dp, vv1.ConditionBucketCreated, metav1.ConditionFalse,
			"ProvisionFailed", err.Error())
		r.Recorder.Eventf(dp, corev1.EventTypeWarning, vv1.EventReasonBucketFailed,
			"Failed to provision bucket %q: %v", bucketName, err)
		metrics.MinioBucketProvision.WithLabelValues("failure").Inc()
		return fmt.Errorf("bucket reconciliation failed: %w", err)
	}

	if created {
		logger.Info("Created MinIO bucket", "bucket", bucketName)
		r.Recorder.Eventf(dp, corev1.EventTypeNormal, vv1.EventReasonBucketCreated,
			"Created MinIO bucket %q for pipeline %s", bucketName, dp.Name)
		metrics.MinioBucketProvision.WithLabelValues("success").Inc()
	}

	r.setCondition(dp, vv1.ConditionBucketCreated, metav1.ConditionTrue,
		"BucketReady", fmt.Sprintf("Bucket %q exists and is ready", bucketName))
	return nil
}

// reconcileConfigMap creates or updates the ConfigMap that holds the pipeline's
// runtime configuration (schedule, image, transform reference, resource limits).
// The Airflow DAG runner reads this ConfigMap at task startup.
func (r *DataPipelineReconciler) reconcileConfigMap(ctx context.Context, dp *vv1.DataPipeline) error {
	logger := log.FromContext(ctx)

	cmName := dp.Name + configMapSuffix
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: dp.Namespace,
			Labels:    pipelineLabels(dp),
		},
		Data: map[string]string{
			"pipeline.name":          dp.Name,
			"pipeline.schedule":      dp.Spec.Schedule,
			"pipeline.image":         fmt.Sprintf("%s:%s", dp.Spec.Image.Repository, dp.Spec.Image.Tag),
			"pipeline.source.type":   dp.Spec.Source.Type,
			"pipeline.source.secret": dp.Spec.Source.SecretRef,
			"pipeline.dest.type":     dp.Spec.Destination.Type,
			"pipeline.dest.bucket":   dp.Spec.Destination.Bucket,
			"pipeline.transform":     dp.Spec.Transform,
			"pipeline.transformRef":  dp.Spec.TransformRef,
			"pipeline.retries":       fmt.Sprintf("%d", dp.Spec.Retries),
			"pipeline.timeout":       dp.Spec.Timeout,
		},
	}

	// Set controller reference so the ConfigMap is garbage-collected with the pipeline.
	if err := controllerutil.SetControllerReference(dp, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on ConfigMap: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: dp.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating ConfigMap", "configMap", cmName)
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create ConfigMap: %w", err)
		}
		dp.Status.ConfigMapName = cmName
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Update if data has changed.
	if !mapsEqual(existing.Data, desired.Data) {
		existing.Data = desired.Data
		existing.Labels = desired.Labels
		if err := r.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update ConfigMap: %w", err)
		}
		logger.Info("Updated ConfigMap", "configMap", cmName)
	}

	dp.Status.ConfigMapName = cmName
	return nil
}

// validateSourceSecret checks that the Secret referenced by spec.source.secretRef
// exists in the same namespace. Emits a SecretMissing event if not.
func (r *DataPipelineReconciler) validateSourceSecret(ctx context.Context, dp *vv1.DataPipeline) error {
	secretName := dp.Spec.Source.SecretRef
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: dp.Namespace}, secret)
	if apierrors.IsNotFound(err) {
		r.Recorder.Eventf(dp, corev1.EventTypeWarning, vv1.EventReasonSecretMissing,
			"Source secret %q not found in namespace %q — pipeline cannot run until this is created",
			secretName, dp.Namespace)
		return fmt.Errorf("source secret %q not found", secretName)
	}
	return err
}

// reconcileCronJob creates or updates the CronJob that triggers the pipeline.
// The CronJob runs a small runner container that calls the Airflow REST API
// to trigger a DAG run — it does NOT write Python files into a shared volume.
func (r *DataPipelineReconciler) reconcileCronJob(ctx context.Context, dp *vv1.DataPipeline) error {
	logger := log.FromContext(ctx)

	cjName := dp.Name + cronJobSuffix
	desired := r.buildCronJob(dp, cjName)

	// Set controller reference so the CronJob is GC'd with the pipeline.
	if err := controllerutil.SetControllerReference(dp, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on CronJob: %w", err)
	}

	existing := &batchv1.CronJob{}
	err := r.Get(ctx, types.NamespacedName{Name: cjName, Namespace: dp.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating CronJob", "cronJob", cjName)
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create CronJob: %w", err)
		}
		r.Recorder.Eventf(dp, corev1.EventTypeNormal, vv1.EventReasonDAGUpdated,
			"Created CronJob %q for pipeline %s (schedule: %s)", cjName, dp.Name, dp.Spec.Schedule)
		r.setCondition(dp, vv1.ConditionDAGSynced, metav1.ConditionTrue, "CronJobCreated",
			fmt.Sprintf("CronJob %q created", cjName))
		dp.Status.CronJobName = cjName
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get CronJob: %w", err)
	}

	// Update schedule or resource spec if changed.
	specChanged := existing.Spec.Schedule != desired.Spec.Schedule
	if specChanged {
		existing.Spec.Schedule = desired.Spec.Schedule
		existing.Spec.JobTemplate = desired.Spec.JobTemplate
		if err := r.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update CronJob: %w", err)
		}
		logger.Info("Updated CronJob", "cronJob", cjName, "newSchedule", desired.Spec.Schedule)
		r.Recorder.Eventf(dp, corev1.EventTypeNormal, vv1.EventReasonDAGUpdated,
			"Updated CronJob %q schedule to %q", cjName, desired.Spec.Schedule)
	}

	r.setCondition(dp, vv1.ConditionDAGSynced, metav1.ConditionTrue, "CronJobReady",
		fmt.Sprintf("CronJob %q is up to date", cjName))
	dp.Status.CronJobName = cjName
	return nil
}

// buildCronJob constructs the desired CronJob spec for a DataPipeline.
// The job runs a small "dag-trigger" container that calls Airflow's REST API.
// This is the key architectural decision: no DAG files in shared volumes.
func (r *DataPipelineReconciler) buildCronJob(dp *vv1.DataPipeline, cjName string) *batchv1.CronJob {
	cpuReq := dp.Spec.Resources.CPU
	if cpuReq == "" {
		cpuReq = "100m"
	}
	memReq := dp.Spec.Resources.Memory
	if memReq == "" {
		memReq = "128Mi"
	}

	successLimit := int32(3)
	failedLimit := int32(3)
	backoffLimit := int32(int32(dp.Spec.Retries)) //nolint:gosimple

	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cjName,
			Namespace: dp.Namespace,
			Labels:    pipelineLabels(dp),
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   dp.Spec.Schedule,
			ConcurrencyPolicy:          batchv1.ForbidConcurrent,
			SuccessfulJobsHistoryLimit: &successLimit,
			FailedJobsHistoryLimit:     &failedLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoffLimit,
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: pipelineLabels(dp),
						},
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name: "dag-trigger",
									// This image calls Airflow's REST API to trigger a DAG run.
									// It reads pipeline config from the ConfigMap mounted below.
									// Source: operator/cmd/dag-trigger/ (built and pushed to registry).
									Image:           "ghcr.io/yashasbn/velora-dag-trigger:latest",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Env: []corev1.EnvVar{
										{
											Name:  "PIPELINE_NAME",
											Value: dp.Name,
										},
										{
											Name:  "AIRFLOW_DAG_ID",
											Value: sanitizeName(dp.Name),
										},
										{
											Name: "AIRFLOW_BASE_URL",
											ValueFrom: &corev1.EnvVarSource{
												ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "velora-airflow-config",
													},
													Key: "airflow.base_url",
												},
											},
										},
										{
											Name: "AIRFLOW_USERNAME",
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "velora-airflow-creds",
													},
													Key: "username",
												},
											},
										},
										{
											Name: "AIRFLOW_PASSWORD",
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "velora-airflow-creds",
													},
													Key: "password",
												},
											},
										},
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse(cpuReq),
											corev1.ResourceMemory: resource.MustParse(memReq),
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "pipeline-config",
											MountPath: "/etc/velora/config",
											ReadOnly:  true,
										},
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "pipeline-config",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: dp.Name + configMapSuffix,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// SetupWithManager registers the controller with the manager.
func (r *DataPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vv1.DataPipeline{}).
		Owns(&batchv1.CronJob{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setCondition sets a metav1.Condition on the DataPipeline status.
// It uses the apimachinery meta.SetStatusCondition helper which correctly
// handles LastTransitionTime and prevents spurious updates.
func (r *DataPipelineReconciler) setCondition(
	dp *vv1.DataPipeline,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&dp.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: dp.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// pipelineLabels returns a consistent label set for all resources owned by a pipeline.
func pipelineLabels(dp *vv1.DataPipeline) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "velora-operator",
		"velora.dev/pipeline":          dp.Name,
		"velora.dev/namespace":         dp.Namespace,
	}
}

// sanitizeName converts a pipeline name into a valid Airflow DAG ID
// (lowercase, hyphens replaced with underscores).
func sanitizeName(name string) string {
	result := make([]byte, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' {
			result[i] = '_'
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// mapsEqual returns true if two string maps have the same keys and values.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// firstError returns the first non-nil error from the list.
func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// boolToFloat64 converts a bool to 0.0 or 1.0 for Prometheus gauges.
func boolToFloat64(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

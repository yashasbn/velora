/*
Copyright 2026 Velora Authors.
Licensed under the Apache License, Version 2.0.
*/

package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vv1 "github.com/yashasbn/velora/api/v1alpha1"
)

// Timeout and polling interval for Eventually/Consistently blocks.
const (
	timeout  = 30 * time.Second
	interval = 250 * time.Millisecond
)

var _ = Describe("DataPipeline Controller", func() {
	ctx := context.Background()

	// Helper: create a minimal DataPipeline for each test.
	newPipeline := func(name, bucket, secretRef string) *vv1.DataPipeline {
		return &vv1.DataPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: vv1.DataPipelineSpec{
				Schedule: "0 2 * * *",
				Image: vv1.ImageSpec{
					Repository: "apache/airflow",
					Tag:        "2.9.3",
				},
				Source: vv1.SourceSpec{
					Type:      "postgres",
					SecretRef: secretRef,
				},
				Destination: vv1.DestinationSpec{
					Type:   "minio",
					Bucket: bucket,
				},
				Transform:    "sql",
				TransformRef: "transforms/daily-sales.sql",
				Retries:      3,
				Timeout:      "30m",
			},
		}
	}

	// Helper: create a Secret so secret validation passes.
	createSecret := func(name, namespace string) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"host":     []byte("postgres.default.svc.cluster.local"),
				"port":     []byte("5432"),
				"username": []byte("airflow"),
				"password": []byte("secret"),
				"database": []byte("sales"),
			},
		}
		Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	}

	// -------------------------------------------------------------------------
	Describe("Creating a DataPipeline", func() {
		It("should add the finalizer on creation", func() {
			dp := newPipeline("test-finalizer-added", "test-bucket-finalizer", "test-secret-1")
			createSecret("test-secret-1", "default")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			Eventually(func() bool {
				updated := &vv1.DataPipeline{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				for _, f := range updated.Finalizers {
					if f == vv1.FinalizerName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "finalizer should be added")
		})

		It("should provision the MinIO bucket (BucketCreated condition = True)", func() {
			createSecret("test-secret-bucket", "default")
			dp := newPipeline("test-bucket-created", "test-bucket-for-pipeline", "test-secret-bucket")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			Eventually(func() bool {
				updated := &vv1.DataPipeline{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				cond := meta.FindStatusCondition(updated.Status.Conditions, vv1.ConditionBucketCreated)
				return cond != nil && cond.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue(), "BucketCreated condition should be True")

			// Verify the bucket was actually created in the fake client.
			Expect(fakeMinioClient.Buckets["test-bucket-for-pipeline"]).To(BeTrue())
		})

		It("should create a ConfigMap with pipeline configuration", func() {
			createSecret("test-secret-cm", "default")
			dp := newPipeline("test-configmap", "test-bucket-cm", "test-secret-cm")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-configmap-pipeline-config",
					Namespace: "default",
				}, cm)
				return err == nil
			}, timeout, interval).Should(BeTrue(), "ConfigMap should be created")

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-configmap-pipeline-config",
				Namespace: "default",
			}, cm)).Should(Succeed())

			Expect(cm.Data["pipeline.name"]).To(Equal("test-configmap"))
			Expect(cm.Data["pipeline.schedule"]).To(Equal("0 2 * * *"))
			Expect(cm.Data["pipeline.dest.bucket"]).To(Equal("test-bucket-cm"))
		})

		It("should create a CronJob with the correct schedule", func() {
			createSecret("test-secret-cron", "default")
			dp := newPipeline("test-cronjob", "test-bucket-cron", "test-secret-cron")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			Eventually(func() bool {
				cj := &batchv1.CronJob{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-cronjob-pipeline-cron",
					Namespace: "default",
				}, cj)
				return err == nil
			}, timeout, interval).Should(BeTrue(), "CronJob should be created")

			cj := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-cronjob-pipeline-cron",
				Namespace: "default",
			}, cj)).Should(Succeed())

			Expect(cj.Spec.Schedule).To(Equal("0 2 * * *"))
		})

		It("should reach Ready phase when all resources reconcile", func() {
			createSecret("test-secret-ready", "default")
			dp := newPipeline("test-ready-phase", "test-bucket-ready", "test-secret-ready")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			Eventually(func() string {
				updated := &vv1.DataPipeline{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(vv1.PhaseReady))
		})

		It("should set Ready condition to True when all sub-resources are reconciled", func() {
			createSecret("test-secret-cond", "default")
			dp := newPipeline("test-ready-condition", "test-bucket-cond", "test-secret-cond")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			Eventually(func() bool {
				updated := &vv1.DataPipeline{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				cond := meta.FindStatusCondition(updated.Status.Conditions, vv1.ConditionReady)
				return cond != nil && cond.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue(), "Ready condition should be True")
		})
	})

	// -------------------------------------------------------------------------
	Describe("Updating a DataPipeline", func() {
		It("should update the CronJob when the schedule changes", func() {
			createSecret("test-secret-update", "default")
			dp := newPipeline("test-schedule-update", "test-bucket-update", "test-secret-update")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			// Wait for initial CronJob to be created.
			Eventually(func() bool {
				cj := &batchv1.CronJob{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: "test-schedule-update-pipeline-cron", Namespace: "default",
				}, cj) == nil
			}, timeout, interval).Should(BeTrue())

			// Update the schedule.
			updated := &vv1.DataPipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)).Should(Succeed())
			updated.Spec.Schedule = "30 3 * * *"
			Expect(k8sClient.Update(ctx, updated)).Should(Succeed())

			// Verify CronJob schedule is updated.
			Eventually(func() string {
				cj := &batchv1.CronJob{}
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name: "test-schedule-update-pipeline-cron", Namespace: "default",
				}, cj)
				return cj.Spec.Schedule
			}, timeout, interval).Should(Equal("30 3 * * *"))
		})
	})

	// -------------------------------------------------------------------------
	Describe("Missing Source Secret", func() {
		It("should emit a SecretMissing event when the source secret does not exist", func() {
			// Note: do NOT create the secret for this test.
			dp := newPipeline("test-missing-secret", "test-bucket-nosecret", "this-secret-does-not-exist")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			// Pipeline should enter Failed phase because secret is missing.
			Eventually(func() string {
				updated := &vv1.DataPipeline{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(vv1.PhaseFailed))

			// Check that a Warning event was emitted.
			Eventually(func() bool {
				events := &corev1.EventList{}
				_ = k8sClient.List(ctx, events, &client.ListOptions{Namespace: "default"})
				for _, e := range events.Items {
					if e.Reason == vv1.EventReasonSecretMissing &&
						e.InvolvedObject.Name == dp.Name {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "SecretMissing event should be emitted")
		})
	})

	// -------------------------------------------------------------------------
	Describe("Deleting a DataPipeline", func() {
		It("should clean up CronJob and ConfigMap when the pipeline is deleted", func() {
			createSecret("test-secret-delete", "default")
			dp := newPipeline("test-deletion", "test-bucket-del", "test-secret-delete")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			// Wait for resources to be created.
			Eventually(func() bool {
				cj := &batchv1.CronJob{}
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: "test-deletion-pipeline-cron", Namespace: "default",
				}, cj) == nil
			}, timeout, interval).Should(BeTrue())

			// Delete the pipeline.
			Expect(k8sClient.Delete(ctx, dp)).Should(Succeed())

			// CronJob should be removed.
			Eventually(func() bool {
				cj := &batchv1.CronJob{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "test-deletion-pipeline-cron", Namespace: "default",
				}, cj)
				return err != nil // NotFound = deleted
			}, timeout, interval).Should(BeTrue(), "CronJob should be deleted")

			// ConfigMap should be removed.
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: "test-deletion-pipeline-config", Namespace: "default",
				}, cm)
				return err != nil // NotFound = deleted
			}, timeout, interval).Should(BeTrue(), "ConfigMap should be deleted")
		})

		It("should remove the finalizer after cleanup", func() {
			createSecret("test-secret-fin-cleanup", "default")
			dp := newPipeline("test-finalizer-cleanup", "test-bucket-fin", "test-secret-fin-cleanup")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			// Wait for finalizer to be added.
			Eventually(func() bool {
				updated := &vv1.DataPipeline{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				for _, f := range updated.Finalizers {
					if f == vv1.FinalizerName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, dp)).Should(Succeed())

			// Pipeline should eventually be fully deleted (finalizer removed by GC).
			Eventually(func() bool {
				updated := &vv1.DataPipeline{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				return err != nil // resource gone
			}, timeout, interval).Should(BeTrue(), "DataPipeline should be fully deleted")
		})
	})

	// -------------------------------------------------------------------------
	Describe("Bucket provisioning error handling", func() {
		It("should enter Failed phase and set BucketCreated=False when MinIO errors", func() {
			createSecret("test-secret-minio-err", "default")

			// Inject an error into the fake MinIO client.
			fakeMinioClient.EnsureError = fmt.Errorf("minio connection refused")
			DeferCleanup(func() { fakeMinioClient.EnsureError = nil })

			dp := newPipeline("test-minio-error", "test-bucket-err", "test-secret-minio-err")
			Expect(k8sClient.Create(ctx, dp)).Should(Succeed())

			Eventually(func() string {
				updated := &vv1.DataPipeline{}
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)
				return updated.Status.Phase
			}, timeout, interval).Should(Equal(vv1.PhaseFailed))

			updated := &vv1.DataPipeline{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: dp.Name, Namespace: "default"}, updated)).Should(Succeed())
			cond := meta.FindStatusCondition(updated.Status.Conditions, vv1.ConditionBucketCreated)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		})
	})
})

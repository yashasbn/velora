/*
Copyright 2026 Velora Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition type constants — used in DataPipelineStatus.Conditions.
const (
	// ConditionReady is True when all sub-resources are reconciled and the
	// pipeline is running on schedule.
	ConditionReady = "Ready"

	// ConditionBucketCreated is True when the destination MinIO bucket exists.
	ConditionBucketCreated = "BucketCreated"

	// ConditionDAGSynced is True when the Airflow DAG has been registered and
	// the CronJob is up to date.
	ConditionDAGSynced = "DAGSynced"
)

// Phase constants for DataPipelineStatus.Phase.
const (
	PhasePending = "Pending"
	PhaseRunning = "Running"
	PhaseFailed  = "Failed"
	PhaseReady   = "Ready"
)

// Finalizer name — added to every DataPipeline on creation, removed after
// cleanup on deletion.
const FinalizerName = "velora.dev/finalizer"

// Event reason constants — used with recorder.Event() calls.
const (
	EventReasonBucketCreated = "CreatedBucket"
	EventReasonBucketFailed  = "BucketProvisionFailed"
	EventReasonDAGUpdated    = "DAGUpdated"
	EventReasonDAGFailed     = "DAGFailed"
	EventReasonSecretMissing = "SecretMissing"
	EventReasonCleanedUp     = "CleanedUp"
	EventReasonReconciling   = "Reconciling"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=dp;dpipe,categories=velora
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Last Run",type=date,JSONPath=`.status.lastRun`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=='Ready')].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DataPipeline is the Schema for the datapipelines API.
// A single DataPipeline resource represents a declarative data pipeline:
// the platform provisions storage, schedules orchestration, and monitors
// execution automatically.
type DataPipeline struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataPipelineSpec   `json:"spec,omitempty"`
	Status DataPipelineStatus `json:"status,omitempty"`
}

// DataPipelineSpec defines the desired state of DataPipeline.
type DataPipelineSpec struct {
	// Schedule is a cron expression defining when the pipeline runs.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(@(annually|yearly|monthly|weekly|daily|hourly|reboot))|(@every (\d+(ns|us|µs|ms|s|m|h))+)|((((\d+,)+\d+|(\d+(\/|-)\d+)|\d+|\*) ?){5,7})$`
	Schedule string `json:"schedule"`

	// Image specifies the container image used for pipeline execution tasks.
	// +kubebuilder:validation:Required
	Image ImageSpec `json:"image"`

	// Source defines where the pipeline reads data from.
	// +kubebuilder:validation:Required
	Source SourceSpec `json:"source"`

	// Destination defines where the pipeline writes data to.
	// +kubebuilder:validation:Required
	Destination DestinationSpec `json:"destination"`

	// Transform specifies the transformation type (e.g., "sql", "python").
	// +kubebuilder:validation:Enum=sql;python;spark
	// +optional
	Transform string `json:"transform,omitempty"`

	// TransformRef is a reference to the transform logic (e.g., a ConfigMap key path).
	// +optional
	TransformRef string `json:"transformRef,omitempty"`

	// Resources defines the compute resources for pipeline task pods.
	// +optional
	Resources ResourceSpec `json:"resources,omitempty"`

	// Retries is the number of times to retry a failed pipeline run.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=3
	// +optional
	Retries int `json:"retries,omitempty"`

	// Timeout is the maximum duration for a single pipeline run (e.g., "30m", "2h").
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// Alerting configures alerting for pipeline failures.
	// +optional
	Alerting AlertingSpec `json:"alerting,omitempty"`

	// Monitoring enables Prometheus metrics and Grafana dashboard integration.
	// +optional
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`
}

// ImageSpec defines the container image for pipeline execution.
type ImageSpec struct {
	// Repository is the image repository (e.g., "apache/airflow").
	// +kubebuilder:validation:Required
	Repository string `json:"repository"`

	// Tag is the image tag (e.g., "2.9.3"). Defaults to "latest".
	// +kubebuilder:default=latest
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy is the image pull policy. Defaults to IfNotPresent.
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	// +kubebuilder:default=IfNotPresent
	// +optional
	PullPolicy string `json:"pullPolicy,omitempty"`
}

// SourceSpec defines the data source configuration.
type SourceSpec struct {
	// Type is the source type (e.g., "postgres", "mysql", "s3", "kafka").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=postgres;mysql;s3;kafka;bigquery
	Type string `json:"type"`

	// SecretRef is the name of a Kubernetes Secret in the same namespace
	// containing connection credentials. The operator validates this Secret
	// exists and emits a SecretMissing event if not found.
	// +kubebuilder:validation:Required
	SecretRef string `json:"secretRef"`
}

// DestinationSpec defines the data destination configuration.
type DestinationSpec struct {
	// Type is the destination type (e.g., "minio", "s3", "bigquery").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=minio;s3;bigquery;postgres
	Type string `json:"type"`

	// Bucket is the target storage bucket name.
	// The operator will provision this bucket if it does not exist.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=63
	Bucket string `json:"bucket"`

	// SecretRef optionally references credentials for the destination.
	// For MinIO the operator uses its own internal credentials from the
	// velora-minio-creds Secret in velora-system namespace.
	// +optional
	SecretRef string `json:"secretRef,omitempty"`
}

// ResourceSpec defines compute resources for pipeline task pods.
type ResourceSpec struct {
	// CPU is the CPU resource request/limit (e.g., "500m", "2").
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory is the memory resource request/limit (e.g., "1Gi", "512Mi").
	// +optional
	Memory string `json:"memory,omitempty"`
}

// AlertingSpec configures alerting for pipeline failures.
type AlertingSpec struct {
	// Enabled toggles alerting. When true, Alertmanager rules fire on failures.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Channel optionally specifies a Slack/PagerDuty channel for alerts.
	// +optional
	Channel string `json:"channel,omitempty"`
}

// MonitoringSpec configures Prometheus/Grafana integration.
type MonitoringSpec struct {
	// Enabled toggles monitoring. When true, the operator emits per-pipeline
	// Prometheus metrics and the dashboard shows pipeline-level panels.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// DataPipelineStatus defines the observed state of DataPipeline.
type DataPipelineStatus struct {
	// Phase is the high-level lifecycle phase: Pending, Running, Ready, Failed.
	// +optional
	Phase string `json:"phase,omitempty"`

	// ObservedGeneration is the .metadata.generation that was last reconciled.
	// Use this to detect if the spec changed since the last reconcile.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastRun is the timestamp of the most recent pipeline execution (any result).
	// +optional
	LastRun *metav1.Time `json:"lastRun,omitempty"`

	// LastSuccessfulRun is the timestamp of the most recent successful execution.
	// +optional
	LastSuccessfulRun *metav1.Time `json:"lastSuccessfulRun,omitempty"`

	// Conditions holds the conditions for this DataPipeline.
	// Standard condition types: Ready, BucketCreated, DAGSynced.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// FailureSummary is a plain-English root-cause summary written by the
	// optional AI Failure Summarizer service when phase=Failed.
	// +optional
	FailureSummary string `json:"failureSummary,omitempty"`

	// CronJobName is the name of the managed CronJob resource.
	// +optional
	CronJobName string `json:"cronJobName,omitempty"`

	// ConfigMapName is the name of the managed ConfigMap holding pipeline config.
	// +optional
	ConfigMapName string `json:"configMapName,omitempty"`
}

// +kubebuilder:object:root=true

// DataPipelineList contains a list of DataPipeline resources.
type DataPipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataPipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataPipeline{}, &DataPipelineList{})
}

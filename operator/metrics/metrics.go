/*
Copyright 2026 Velora Authors.
Licensed under the Apache License, Version 2.0.
*/

// Package metrics defines all Prometheus metrics exposed by the Velora operator.
// Metrics are registered once at package init time using the controller-runtime
// metrics registry (which is also the default Prometheus registry).
//
// Exposed on the operator's /metrics endpoint (port 8080 by default).
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ReconcileTotal counts all reconcile loop invocations.
	// Label "result" is "success" or "error".
	ReconcileTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "velora",
			Subsystem: "operator",
			Name:      "reconcile_total",
			Help:      "Total number of DataPipeline reconcile loop invocations.",
		},
	)

	// ReconcileErrors counts reconcile loops that returned an error.
	ReconcileErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "velora",
			Subsystem: "operator",
			Name:      "reconcile_errors_total",
			Help:      "Total number of DataPipeline reconcile errors.",
		},
	)

	// ReconcileDuration measures the time each reconcile loop takes.
	ReconcileDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "velora",
			Subsystem: "operator",
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of DataPipeline reconcile loop in seconds.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
		},
	)

	// PipelineReady is a gauge that is 1.0 when the pipeline is Ready, 0.0 otherwise.
	// Labels: pipeline name, namespace.
	PipelineReady = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "velora",
			Subsystem: "datapipeline",
			Name:      "ready",
			Help:      "1 if the DataPipeline is in Ready phase, 0 otherwise.",
		},
		[]string{"pipeline", "namespace"},
	)

	// PipelineFailures counts the total number of times a pipeline has entered Failed phase.
	// Labels: pipeline name, namespace.
	PipelineFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "velora",
			Subsystem: "datapipeline",
			Name:      "failures_total",
			Help:      "Total number of times a DataPipeline has entered Failed phase.",
		},
		[]string{"pipeline", "namespace"},
	)

	// PipelineLastRunTimestamp records the Unix timestamp of the last pipeline run.
	// Labels: pipeline name, namespace.
	PipelineLastRunTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "velora",
			Subsystem: "datapipeline",
			Name:      "last_run_timestamp",
			Help:      "Unix timestamp of the most recent DataPipeline run.",
		},
		[]string{"pipeline", "namespace"},
	)

	// MinioBucketProvision counts bucket provisioning attempts.
	// Label "result" is "success" or "failure".
	MinioBucketProvision = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "velora",
			Subsystem: "minio",
			Name:      "bucket_provision_total",
			Help:      "Total number of MinIO bucket provisioning attempts.",
		},
		[]string{"result"},
	)
)

func init() {
	// Register all metrics with the controller-runtime metrics registry.
	// This registry is served automatically by the manager's metrics server.
	metrics.Registry.MustRegister(
		ReconcileTotal,
		ReconcileErrors,
		ReconcileDuration,
		PipelineReady,
		PipelineFailures,
		PipelineLastRunTimestamp,
		MinioBucketProvision,
	)
}

/*
Copyright 2026 Velora Authors.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"flag"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	vv1 "github.com/yashasbn/velora/api/v1alpha1"
	"github.com/yashasbn/velora/controllers"
	"github.com/yashasbn/velora/internal/airflow"
	minioclient "github.com/yashasbn/velora/internal/minio"
	_ "github.com/yashasbn/velora/metrics" // side-effect: register metrics
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vv1.AddToScheme(scheme))
	utilruntime.Must(batchv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		probeAddr            string
		minioEndpoint        string
		minioAccessKey       string
		minioSecretKey       string
		minioUseSSL          bool
		airflowBaseURL       string
		airflowUsername      string
		airflowPassword      string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager (required for HA deployments).")
	flag.StringVar(&minioEndpoint, "minio-endpoint", "minio.minio.svc.cluster.local:9000", "MinIO S3 endpoint.")
	flag.StringVar(&minioAccessKey, "minio-access-key", "", "MinIO access key ID.")
	flag.StringVar(&minioSecretKey, "minio-secret-key", "", "MinIO secret access key.")
	flag.BoolVar(&minioUseSSL, "minio-use-ssl", false, "Use SSL for MinIO connection.")
	flag.StringVar(&airflowBaseURL, "airflow-base-url", "http://airflow-webserver.airflow.svc.cluster.local:8080", "Airflow webserver base URL.")
	flag.StringVar(&airflowUsername, "airflow-username", "admin", "Airflow REST API username.")
	flag.StringVar(&airflowPassword, "airflow-password", "", "Airflow REST API password.")

	opts := zap.Options{
		Development: true,
		// Use ISO8601 timestamps for human-readable logs (not scientific notation).
		TimeEncoder: zapcore_iso8601,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Fall back to environment variables when CLI flags are empty.
	// The Helm deployment template injects these as env vars from Secrets.
	if minioAccessKey == "" {
		minioAccessKey = os.Getenv("MINIO_ACCESS_KEY")
	}
	if minioSecretKey == "" {
		minioSecretKey = os.Getenv("MINIO_SECRET_KEY")
	}
	if airflowUsername == "" {
		airflowUsername = os.Getenv("AIRFLOW_USERNAME")
	}
	if airflowPassword == "" {
		airflowPassword = os.Getenv("AIRFLOW_PASSWORD")
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// ---------------------------------------------------------------------------
	// Build MinIO client
	// ---------------------------------------------------------------------------
	mc, err := minioclient.NewClient(minioclient.Config{
		Endpoint:        minioEndpoint,
		AccessKeyID:     minioAccessKey,
		SecretAccessKey: minioSecretKey,
		UseSSL:          minioUseSSL,
	})
	if err != nil {
		setupLog.Error(err, "Failed to create MinIO client")
		os.Exit(1)
	}

	// ---------------------------------------------------------------------------
	// Build Airflow client
	// ---------------------------------------------------------------------------
	ac := airflow.NewClient(airflow.Config{
		BaseURL:  airflowBaseURL,
		Username: airflowUsername,
		Password: airflowPassword,
		Timeout:  30 * time.Second,
	})

	// ---------------------------------------------------------------------------
	// Create the controller manager
	// ---------------------------------------------------------------------------
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "velora-operator-leader-election",
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	// ---------------------------------------------------------------------------
	// Register the DataPipeline reconciler
	// ---------------------------------------------------------------------------
	if err := (&controllers.DataPipelineReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("velora-operator"),
		MinioClient:   mc,
		AirflowClient: ac,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "DataPipeline")
		os.Exit(1)
	}

	// ---------------------------------------------------------------------------
	// Health checks
	// ---------------------------------------------------------------------------
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting Velora operator manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

// zapcore_iso8601 is a zapcore.TimeEncoder that uses ISO8601 format.
// This avoids the unreadable scientific-notation timestamps from the default encoder.
func zapcore_iso8601(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.UTC().Format("2006-01-02T15:04:05.000Z"))
}

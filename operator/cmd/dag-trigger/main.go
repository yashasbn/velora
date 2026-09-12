/*
Copyright 2026 Velora Authors.
Licensed under the Apache License, Version 2.0.
*/

// dag-trigger is a small CLI that triggers an Airflow DAG run via the REST API.
// It is designed to run as a Kubernetes Job/CronJob pod — it does its work and
// exits. The Velora operator's reconciler creates CronJobs that use this image.
//
// Required environment variables:
//
//	AIRFLOW_BASE_URL   — e.g. "http://airflow-webserver.airflow.svc.cluster.local:8080"
//	AIRFLOW_USERNAME   — Airflow basic-auth username
//	AIRFLOW_PASSWORD   — Airflow basic-auth password
//	AIRFLOW_DAG_ID     — the DAG ID to trigger (e.g. "user_events_daily")
//
// Optional:
//
//	PIPELINE_NAME      — human-readable pipeline name (used in log messages)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/yashasbn/velora/internal/airflow"
)

func main() {
	// -------------------------------------------------------------------------
	// Parse environment
	// -------------------------------------------------------------------------
	baseURL := requireEnv("AIRFLOW_BASE_URL")
	username := requireEnv("AIRFLOW_USERNAME")
	password := requireEnv("AIRFLOW_PASSWORD")
	dagID := requireEnv("AIRFLOW_DAG_ID")
	pipelineName := os.Getenv("PIPELINE_NAME")
	if pipelineName == "" {
		pipelineName = dagID
	}

	log.Printf("[dag-trigger] Starting trigger for pipeline=%q dagID=%q airflow=%s",
		pipelineName, dagID, baseURL)

	// -------------------------------------------------------------------------
	// Build Airflow client (reuse the existing internal package)
	// -------------------------------------------------------------------------
	client := airflow.NewClient(airflow.Config{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
		Timeout:  30 * time.Second,
	})

	// -------------------------------------------------------------------------
	// Trigger the DAG run
	// -------------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	run, err := client.TriggerDAGRun(ctx, dagID, map[string]any{
		"triggered_by": "velora-dag-trigger",
		"pipeline":     pipelineName,
	})
	if err != nil {
		log.Fatalf("[dag-trigger] FAILED to trigger DAG %q: %v", dagID, err)
	}

	log.Printf("[dag-trigger] SUCCESS — DAG=%q runID=%q state=%q",
		run.DAGID, run.DagRunID, run.State)
}

// requireEnv fetches an environment variable or fatally exits.
func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		fmt.Fprintf(os.Stderr, "[dag-trigger] FATAL: required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return val
}

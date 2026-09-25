/*
Copyright 2026 Velora Authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
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

/*
Copyright 2026 Velora Authors.
Licensed under the Apache License, Version 2.0.
*/

package airflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is the interface the reconciler and failure summarizer use to
// interact with Airflow's REST API.
// Defined as an interface so tests can substitute a fake.
type Client interface {
	// TriggerDAGRun triggers a new DAG run for the given DAG ID.
	TriggerDAGRun(ctx context.Context, dagID string, conf map[string]any) (*DAGRun, error)

	// GetDAGRunStatus returns the current state of a DAG run.
	GetDAGRunStatus(ctx context.Context, dagID, runID string) (*DAGRun, error)

	// GetTaskLogs returns the log output for a specific task instance.
	// Used by the Failure Summarizer service in Phase 5.
	GetTaskLogs(ctx context.Context, dagID, runID, taskID string, tryNumber int) (string, error)
}

// DAGRun represents an Airflow DAG run response.
type DAGRun struct {
	DAGID        string    `json:"dag_id"`
	DagRunID     string    `json:"dag_run_id"`
	State        string    `json:"state"`   // "queued", "running", "success", "failed"
	LogicalDate  time.Time `json:"logical_date"`
	StartDate    *time.Time `json:"start_date,omitempty"`
	EndDate      *time.Time `json:"end_date,omitempty"`
}

// httpClient implements Client via Airflow's REST API.
type httpClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// Config holds connection settings for the Airflow client.
type Config struct {
	BaseURL  string // e.g. "http://airflow-webserver.airflow.svc.cluster.local:8080"
	Username string
	Password string
	Timeout  time.Duration
}

// NewClient creates a new Airflow REST API client.
func NewClient(cfg Config) Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &httpClient{
		baseURL:  cfg.BaseURL,
		username: cfg.Username,
		password: cfg.Password,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// TriggerDAGRun triggers a DAG run via POST /api/v1/dags/{dag_id}/dagRuns.
// This is how the Velora operator drives Airflow — it never writes Python
// files to a shared volume.
func (c *httpClient) TriggerDAGRun(ctx context.Context, dagID string, conf map[string]any) (*DAGRun, error) {
	url := fmt.Sprintf("%s/api/v1/dags/%s/dagRuns", c.baseURL, dagID)

	body := map[string]any{
		"dag_run_id": fmt.Sprintf("velora_%d", time.Now().Unix()),
		"conf":       conf,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling dag run request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("triggering dag run: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("airflow returned %d: %s", resp.StatusCode, string(b))
	}

	var run DAGRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, fmt.Errorf("decoding dag run response: %w", err)
	}
	return &run, nil
}

// GetDAGRunStatus fetches the status of a DAG run via GET /api/v1/dags/{dag_id}/dagRuns/{run_id}.
func (c *httpClient) GetDAGRunStatus(ctx context.Context, dagID, runID string) (*DAGRun, error) {
	url := fmt.Sprintf("%s/api/v1/dags/%s/dagRuns/%s", c.baseURL, dagID, runID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting dag run status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("airflow returned %d: %s", resp.StatusCode, string(b))
	}

	var run DAGRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &run, nil
}

// GetTaskLogs fetches task log output via GET /api/v1/dags/{dag_id}/dagRuns/{run_id}/taskInstances/{task_id}/logs/{try_number}.
// Used by the Phase 5 Failure Summarizer.
func (c *httpClient) GetTaskLogs(ctx context.Context, dagID, runID, taskID string, tryNumber int) (string, error) {
	url := fmt.Sprintf("%s/api/v1/dags/%s/dagRuns/%s/taskInstances/%s/logs/%d",
		c.baseURL, dagID, runID, taskID, tryNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching task logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("airflow returned %d", resp.StatusCode)
	}

	logs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading log body: %w", err)
	}
	return string(logs), nil
}

// ---------------------------------------------------------------------------
// Fake client for testing
// ---------------------------------------------------------------------------

// FakeClient is an in-memory Airflow client for unit tests.
type FakeClient struct {
	TriggerError error
	StatusError  error
	LogsContent  string
	Runs         map[string]*DAGRun
}

// NewFakeClient creates a new FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{Runs: make(map[string]*DAGRun)}
}

func (f *FakeClient) TriggerDAGRun(_ context.Context, dagID string, _ map[string]any) (*DAGRun, error) {
	if f.TriggerError != nil {
		return nil, f.TriggerError
	}
	run := &DAGRun{DAGID: dagID, DagRunID: "fake-run-id", State: "queued"}
	f.Runs[dagID] = run
	return run, nil
}

func (f *FakeClient) GetDAGRunStatus(_ context.Context, dagID, _ string) (*DAGRun, error) {
	if f.StatusError != nil {
		return nil, f.StatusError
	}
	if run, ok := f.Runs[dagID]; ok {
		return run, nil
	}
	return nil, fmt.Errorf("run not found")
}

func (f *FakeClient) GetTaskLogs(_ context.Context, _, _, _ string, _ int) (string, error) {
	return f.LogsContent, nil
}

# Velora — GitOps Data Platform

Declarative data pipeline provisioning and reconciliation on Kubernetes.

## Developer Experience

**Before Velora**
1. Create and test an Apache Airflow DAG in Python.
2. Manually provision target storage buckets (e.g. S3, GCS, MinIO).
3. Securely share connection secrets (e.g., PostgreSQL credentials) to the DAG.
4. Manually configure retry parameters, schedules, and timeouts.
5. Set up dashboard metrics, alerts, and SLOs.
6. Deploy the DAG to shared volume / Git repository.
7. Verify all components are wired correctly.

**After Velora**
```bash
kubectl apply -f pipeline.yaml
```

Applying this single file handles bucket provisioning, configuration propagation, scheduling, and monitoring automatically.

---

## Tech Stack
- **Cluster**: `kind` (Kubernetes in Docker) for local development, `GKE` for cloud.
- **IaC**: Terraform to provision local `kind` and cloud-optional infrastructure.
- **GitOps**: ArgoCD to manage Git-to-cluster synchronization.
- **Operator**: Custom Go operator built with `kubebuilder` (v4).
- **Orchestration**: Apache Airflow driven programmatically via REST API (KubernetesExecutor).
- **Storage**: MinIO (S3-compatible, self-hosted) for pipeline outputs.
- **Observability**: Prometheus + Grafana stack with custom metrics.

---

## Architecture

```mermaid
graph TD
    %% Define styles/colors
    classDef git fill:#F05032,stroke:#333,stroke-width:2px,color:#fff;
    classDef k8s fill:#326CE5,stroke:#333,stroke-width:2px,color:#fff;
    classDef operator fill:#00ADD8,stroke:#333,stroke-width:2px,color:#fff;
    classDef workload fill:#F5A623,stroke:#333,stroke-width:2px,color:#fff;
    classDef obs fill:#E6522C,stroke:#333,stroke-width:2px,color:#fff;

    %% Nodes
    GitPush["Git Push"] --> Github["GitHub Repository"]:::git
    Github --> ArgoCD["ArgoCD Sync"]:::git
    ArgoCD --> K8sAPI["Kubernetes API Server"]:::k8s
    K8sAPI --> CRD["DataPipeline CRD"]:::k8s
    CRD --> Operator["Velora Operator"]:::operator

    subgraph Provisioning ["Platform Reconciliation (Operator Loop)"]
        Operator --> MinIO["MinIO Buckets"]:::workload
        Operator --> Config["ConfigMaps / Secrets"]:::workload
        Operator --> Cron["CronJob / Airflow REST API"]:::workload
    end

    subgraph Runtime ["Pipeline Execution"]
        Cron --> AirflowRun["Airflow Pipeline Run"]:::workload
        Config --> AirflowRun
        MinIO --> AirflowRun
    end

    subgraph Telemetry ["Observability Stack"]
        Operator --> PromMetrics["Prometheus Metrics"]:::obs
        AirflowRun --> PromMetrics
        PromMetrics --> Grafana["Grafana Dashboard"]:::obs
        PromMetrics --> Alertmanager["Alertmanager Alerts"]:::obs
    end

    subgraph AI ["AI Root Cause Analysis"]
        AirflowRun -- "On Failure" --> FailureSvc["Failure Summarizer Service"]:::obs
        FailureSvc --> LLM["LLM (Gemini / Ollama)"]
        LLM --> UpdateStatus["Update status.failureSummary"]:::k8s
        UpdateStatus --> CRD
    end
```

---


## Setup & Bootstrap (Phase 1)

Ensure you have completed the [WSL2 Ubuntu Prerequisite Setup](docs/implementation_plan.md) (Docker, Go 1.22+, Terraform, Helm, kubectl, kind, and kubebuilder installed inside WSL2).

### 1. Initialize & Start the Platform
Run the bootstrap script inside your WSL2 environment:
```bash
chmod +x scripts/bootstrap.sh
./scripts/bootstrap.sh
```

### 2. Access Dashboards
To forward all platform services, run:
```bash
./scripts/port-forward.sh all
```

Access URLs:
- **ArgoCD**: [http://localhost:8080](http://localhost:8080) (admin / auto-generated password)
- **Airflow**: [http://localhost:8081](http://localhost:8081)
- **MinIO**: [http://localhost:9001](http://localhost:9001) (velora / velora-minio-secret)
- **Grafana**: [http://localhost:3000](http://localhost:3000)

#!/usr/bin/env bash
# =============================================================================
# scripts/verify-phase.sh
# Phase verification helper — checks what "done" looks like for each phase.
#
# Usage:
#   ./scripts/verify-phase.sh <phase_number>   # e.g. ./scripts/verify-phase.sh 1
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

PHASE="${1:-}"
[[ -z "$PHASE" ]] && { echo "Usage: $0 <phase>  (1–6)"; exit 1; }

KUBECONFIG="${KUBECONFIG:-$HOME/.kube/velora-config}"
export KUBECONFIG

PASS=0; FAIL=0

check() {
  local desc="$1"; shift
  if "$@" &>/dev/null; then
    echo -e "  ${GREEN}✔${NC}  $desc"
    ((PASS++)) || true
  else
    echo -e "  ${RED}✘${NC}  $desc"
    ((FAIL++)) || true
  fi
}

check_output() {
  local desc="$1" pattern="$2"; shift 2
  if "$@" 2>/dev/null | grep -q "$pattern"; then
    echo -e "  ${GREEN}✔${NC}  $desc"
    ((PASS++)) || true
  else
    echo -e "  ${RED}✘${NC}  $desc  ${YELLOW}(pattern: '$pattern')${NC}"
    ((FAIL++)) || true
  fi
}

separator() { echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"; }

# ---------------------------------------------------------------------------
phase1() {
  separator
  echo -e "${BOLD}Phase 1 — Foundation${NC}"
  separator

  check "Docker daemon running"                   docker info
  check "kind CLI available"                      kind version
  check "kubectl available"                       kubectl version --client
  check "Cluster 'velora' exists"                 kind get clusters
  check_output "All nodes Ready"  "Ready"         kubectl get nodes --no-headers
  check_output "ArgoCD pods Running" "Running"    kubectl get pods -n argocd --no-headers
  check "ArgoCD server deployment available"      kubectl rollout status deployment/argocd-server -n argocd --timeout=10s
}

# ---------------------------------------------------------------------------
phase2() {
  separator
  echo -e "${BOLD}Phase 2 — Core Workload${NC}"
  separator

  check_output "Airflow webserver Running" "Running"  kubectl get pods -n airflow -l component=webserver --no-headers
  check_output "Airflow scheduler Running" "Running"  kubectl get pods -n airflow -l component=scheduler --no-headers
  check_output "MinIO pod Running"         "Running"  kubectl get pods -n minio --no-headers
  check_output "MinIO service exists"      "minio"    kubectl get svc -n minio --no-headers
  check "Airflow webserver deployment ready"      kubectl rollout status deployment/airflow-webserver -n airflow --timeout=10s
}

# ---------------------------------------------------------------------------
phase3() {
  separator
  echo -e "${BOLD}Phase 3 — Operator${NC}"
  separator

  check "DataPipeline CRD installed"              kubectl get crd datapipelines.velora.dev
  check_output "Operator pod Running" "Running"   kubectl get pods -n velora-system --no-headers
  check "Operator deployment ready"               kubectl rollout status deployment/velora-operator -n velora-system --timeout=10s

  # Check example pipelines
  for pipeline in daily-sales-etl hourly-events-etl; do
    check_output "${pipeline} phase=Ready" "Ready" \
      kubectl get datapipeline "$pipeline" -o jsonpath='{.status.phase}'
    check_output "${pipeline} BucketCreated=True" "True" \
      kubectl get datapipeline "$pipeline" -o jsonpath='{.status.conditions[?(@.type=="BucketCreated")].status}'
    check_output "${pipeline} DAGSynced=True" "True" \
      kubectl get datapipeline "$pipeline" -o jsonpath='{.status.conditions[?(@.type=="DAGSynced")].status}'
  done
}

# ---------------------------------------------------------------------------
phase4() {
  separator
  echo -e "${BOLD}Phase 4 — Observability${NC}"
  separator

  check_output "Prometheus pod Running" "Running"  kubectl get pods -n monitoring -l app.kubernetes.io/name=prometheus --no-headers
  check_output "Grafana pod Running"    "Running"  kubectl get pods -n monitoring -l app.kubernetes.io/name=grafana --no-headers
  check "Operator ServiceMonitor exists"           kubectl get servicemonitor velora-operator -n velora-system
  check "Velora Grafana dashboard ConfigMap exists" kubectl get configmap velora-dashboard -n monitoring
}

# ---------------------------------------------------------------------------
phase5() {
  separator
  echo -e "${BOLD}Phase 5 — Failure Summarizer${NC}"
  separator

  check_output "Summarizer pod Running" "Running"  kubectl get pods -n velora-system -l app=velora-summarizer --no-headers
  echo -e "  ${YELLOW}ℹ${NC}  Manual check: trigger a pipeline failure and verify status.failureSummary gets populated"
}

# ---------------------------------------------------------------------------
phase6() {
  separator
  echo -e "${BOLD}Phase 6 — Polish${NC}"
  separator

  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/.."
  check "README.md exists"              test -f "$REPO_ROOT/README.md"
  check "docs/architecture.md exists"  test -f "$REPO_ROOT/docs/architecture.md"
  check "hack/ansible/bootstrap.yml"   test -f "$REPO_ROOT/hack/ansible/bootstrap.yml"
  check_output "README has Before/After section" "Before Velora" cat "$REPO_ROOT/README.md"
  check_output "README has Future Work section"  "Future Work"   cat "$REPO_ROOT/README.md"
}

# ---------------------------------------------------------------------------
# Dispatch
# ---------------------------------------------------------------------------
case "$PHASE" in
  1) phase1 ;;
  2) phase1; phase2 ;;
  3) phase1; phase2; phase3 ;;
  4) phase1; phase2; phase3; phase4 ;;
  5) phase1; phase2; phase3; phase4; phase5 ;;
  6) phase1; phase2; phase3; phase4; phase5; phase6 ;;
  *) echo "Phase must be 1–6"; exit 1 ;;
esac

separator
echo ""
echo -e "  ${GREEN}Passed: ${PASS}${NC}   ${RED}Failed: ${FAIL}${NC}"
echo ""
if [[ $FAIL -eq 0 ]]; then
  echo -e "  ${GREEN}${BOLD}Phase ${PHASE} verification PASSED ✔${NC}"
else
  echo -e "  ${RED}${BOLD}Phase ${PHASE} verification has ${FAIL} failure(s) — fix before proceeding.${NC}"
  exit 1
fi

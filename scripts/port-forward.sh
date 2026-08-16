#!/usr/bin/env bash
# =============================================================================
# scripts/port-forward.sh
# Quick port-forward helper for all Velora services.
#
# Usage:
#   ./scripts/port-forward.sh <service>
#   ./scripts/port-forward.sh all      # background port-forwards for everything
#
# Services: argocd | airflow | minio | grafana | prometheus | all
# =============================================================================
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
fatal() { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

SERVICE="${1:-all}"
KUBECONFIG="${KUBECONFIG:-$HOME/.kube/velora-config}"
export KUBECONFIG

pf() {
  local name=$1 ns=$2 svc=$3 local_port=$4 remote_port=$5
  info "Port-forwarding ${name}: http://localhost:${local_port}"
  kubectl port-forward "svc/${svc}" "${local_port}:${remote_port}" -n "${ns}" &
  echo $! >> /tmp/velora-pf-pids
}

cleanup() {
  if [[ -f /tmp/velora-pf-pids ]]; then
    info "Stopping port-forwards..."
    while read -r pid; do kill "$pid" 2>/dev/null || true; done < /tmp/velora-pf-pids
    rm -f /tmp/velora-pf-pids
  fi
}
trap cleanup EXIT INT TERM

rm -f /tmp/velora-pf-pids

case "$SERVICE" in
  argocd)
    pf "ArgoCD" argocd argocd-server 8080 80
    info "ArgoCD UI: http://localhost:8080  (user: admin)"
    wait
    ;;
  airflow)
    pf "Airflow" airflow airflow-webserver 8081 8080
    info "Airflow UI: http://localhost:8081  (user: admin)"
    wait
    ;;
  minio)
    pf "MinIO API"     minio minio 9000 9000
    pf "MinIO Console" minio minio-console 9001 9001
    info "MinIO Console: http://localhost:9001"
    wait
    ;;
  grafana)
    pf "Grafana" monitoring prometheus-stack-grafana 3000 80
    info "Grafana: http://localhost:3000  (user: admin, pass: from secret)"
    wait
    ;;
  prometheus)
    pf "Prometheus" monitoring prometheus-stack-kube-prom-prometheus 9090 9090
    info "Prometheus: http://localhost:9090"
    wait
    ;;
  all)
    pf "ArgoCD"        argocd    argocd-server                            8080 80
    pf "Airflow"       airflow   airflow-webserver                        8081 8080
    pf "MinIO API"     minio     minio                                    9000 9000
    pf "MinIO Console" minio     minio-console                            9001 9001
    pf "Grafana"       monitoring prometheus-stack-grafana                3000 80
    pf "Prometheus"    monitoring prometheus-stack-kube-prom-prometheus   9090 9090
    echo ""
    ok "All port-forwards active:"
    echo "  ArgoCD:         http://localhost:8080"
    echo "  Airflow:        http://localhost:8081"
    echo "  MinIO Console:  http://localhost:9001"
    echo "  Grafana:        http://localhost:3000"
    echo "  Prometheus:     http://localhost:9090"
    echo ""
    echo "Press Ctrl+C to stop all port-forwards."
    wait
    ;;
  *)
    fatal "Unknown service: '$SERVICE'. Valid options: argocd | airflow | minio | grafana | prometheus | all"
    ;;
esac

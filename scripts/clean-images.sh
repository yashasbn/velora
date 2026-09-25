#!/usr/bin/env bash
# =============================================================================
# scripts/clean-images.sh
# Removes all Velora pre-downloaded container images from both local Docker host
# and Kind cluster nodes (if cluster is running).
#
# Usage:
#   ./scripts/clean-images.sh
# =============================================================================
set -euo pipefail

CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fatal()   { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

export PATH="$PATH:/snap/bin:/usr/local/go/bin:$HOME/go/bin"
CLUSTER="${CLUSTER_NAME:-velora}"

IMAGES=(
  # ArgoCD components
  "quay.io/argoproj/argocd:v2.14.11"
  "ghcr.io/dexidp/dex:v2.41.1"
  "redis:7.2.4-alpine"

  # MinIO storage
  "cgr.dev/chainguard/minio:latest"

  # Airflow & Database
  "apache/airflow:2.9.3"
  "postgres:16-alpine"

  # Prometheus stack & Grafana
  "quay.io/prometheus/prometheus:v2.53.0"
  "docker.io/grafana/grafana:11.0.0"
  "quay.io/kiwigrid/k8s-sidecar:1.26.1"
  "quay.io/prometheus-operator/prometheus-operator:v0.75.0"
  "quay.io/prometheus-operator/prometheus-config-reloader:v0.75.0"
  "quay.io/prometheus/alertmanager:v0.27.0"
  "quay.io/prometheus/node-exporter:v1.8.1"
  "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.12.0"
)

TOTAL=${#IMAGES[@]}
echo -e "${CYAN}================================================================${NC}"
echo -e "${CYAN}  Velora — Cleaning ${TOTAL} Container Images${NC}"
echo -e "${CYAN}================================================================${NC}"
echo ""

# 1. Remove images from local Docker host
info "Removing images from local Docker host..."
for img in "${IMAGES[@]}"; do
  if docker image inspect "$img" &>/dev/null; then
    docker rmi "$img" 2>/dev/null || true
    success "Removed $img from Docker host."
  else
    info "Skipped $img (not present on Docker host)."
  fi
done

# 2. Remove images from Kind cluster nodes if cluster is running
if command -v kind &>/dev/null && kind get clusters 2>/dev/null | grep -q "^${CLUSTER}$"; then
  info "Kind cluster '${CLUSTER}' detected. Removing images from Kind nodes..."
  NODES=$(kind get nodes --name "${CLUSTER}" 2>/dev/null || true)
  for node in $NODES; do
    info "Cleaning images on node '$node'..."
    for img in "${IMAGES[@]}"; do
      docker exec "$node" crictl rmi "$img" 2>/dev/null || true
    done
    success "Cleaned images on '$node'."
  done
else
  info "Kind cluster '${CLUSTER}' is not running (skipped node cleanup)."
fi

echo ""
success "Image cleanup completed successfully!"

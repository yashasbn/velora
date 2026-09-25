#!/usr/bin/env bash
# =============================================================================
# scripts/load-images.sh
# Loads all locally cached Docker images into the running Kind cluster.
#
# Usage:
#   ./scripts/load-images.sh
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
export KUBECONFIG="$HOME/.kube/velora-config"

CLUSTER="${CLUSTER_NAME:-velora}"

# Check kind cluster
if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER}$"; then
  fatal "Kind cluster '${CLUSTER}' is not running. Start it first with ./scripts/bootstrap.sh"
fi

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
info "Loading ${TOTAL} images into Kind cluster '${CLUSTER}'..."

index=1
for img in "${IMAGES[@]}"; do
  info "[$index/$TOTAL] Loading $img ..."
  # If image is not yet on host, pull it first
  if ! docker image inspect "$img" &>/dev/null; then
    info "  Image not found locally, pulling $img first..."
    docker pull "$img"
  fi
  kind load docker-image "$img" --name "$CLUSTER"
  success "[$index/$TOTAL] Loaded $img into Kind nodes."
  ((index++)) || true
done

echo ""
success "All images loaded into Kind nodes successfully!"
echo "Pods will now start instantly using local image cache."
echo ""
echo "Next step: install ArgoCD & apply App-of-Apps with:"
echo "  ./scripts/install-argocd.sh"

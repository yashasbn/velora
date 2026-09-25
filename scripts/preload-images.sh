#!/usr/bin/env bash
# =============================================================================
# scripts/preload-images.sh
# Pre-pull all container images on the Docker host and load them into the kind
# cluster.  This is necessary because kind's containerd frequently hits TLS
# handshake timeouts when pulling from Docker Hub, quay.io, etc.
#
# Usage:
#   ./scripts/preload-images.sh [cluster_name]
#
# The cluster name defaults to "velora".
# =============================================================================
set -euo pipefail

export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin:/snap/bin

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fatal()   { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

CLUSTER_NAME="${1:-velora}"

# ---------------------------------------------------------------------------
# Image manifest — every image needed for a complete Velora local deployment.
# Update versions here when upgrading Helm charts.
# ---------------------------------------------------------------------------
IMAGES=(
  # -- ArgoCD (chart 10.2.2) --
  "quay.io/argoproj/argocd:v2.14.11"
  "ghcr.io/dexidp/dex:v2.38.0"
  "public.ecr.aws/docker/library/redis:7.2.4-alpine"

  # -- Airflow (chart 1.15.0 / app 2.9.3) --
  "apache/airflow:2.9.3"
  "bitnami/postgresql:16.3.0-debian-12-r23"

  # -- MinIO --
  "minio/minio:latest"

  # -- Prometheus Stack (chart ~61.x) --
  "quay.io/prometheus/prometheus:v2.53.0"
  "quay.io/prometheus-operator/prometheus-operator:v0.75.0"
  "quay.io/prometheus/node-exporter:v1.8.1"
  "quay.io/prometheus/alertmanager:v0.27.0"
  "docker.io/grafana/grafana:11.0.0"
  "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.12.0"

  # -- Velora Operator (built locally — only load if already built) --
  "ghcr.io/yashasbn/velora-operator:latest"
)

# ---------------------------------------------------------------------------
# Step 1: Pull images on Docker host (with retries)
# ---------------------------------------------------------------------------
info "Pulling ${#IMAGES[@]} images on Docker host..."
PULLED=0
SKIPPED=0

for img in "${IMAGES[@]}"; do
  # Skip the velora-operator — it's built locally, not pulled
  if [[ "$img" == *"velora-operator"* ]]; then
    if docker image inspect "$img" &>/dev/null; then
      info "  [local] ${img} — already built"
    else
      warn "  [skip]  ${img} — not built yet (run 'make docker-build' in operator/)"
      ((SKIPPED++)) || true
    fi
    continue
  fi

  # Skip if already cached on Docker host
  if docker image inspect "$img" &>/dev/null; then
    info "  [cache] ${img}"
    ((PULLED++)) || true
    continue
  fi

  # Pull with 3 retries
  for attempt in 1 2 3; do
    if docker pull "$img" 2>/dev/null; then
      success "  [pull]  ${img}"
      ((PULLED++)) || true
      break
    fi
    if [[ $attempt -eq 3 ]]; then
      warn "  [FAIL]  ${img} — could not pull after 3 attempts"
      ((SKIPPED++)) || true
    else
      warn "  Retry ${attempt}/3 for ${img}..."
      sleep 5
    fi
  done
done

success "Pulled/cached: ${PULLED}, skipped: ${SKIPPED}"

# ---------------------------------------------------------------------------
# Step 2: Load images into kind cluster
# ---------------------------------------------------------------------------
info "Loading images into kind cluster '${CLUSTER_NAME}'..."
LOADED=0

for img in "${IMAGES[@]}"; do
  # Only load images that exist locally
  if ! docker image inspect "$img" &>/dev/null; then
    continue
  fi

  if kind load docker-image "$img" --name "${CLUSTER_NAME}" 2>/dev/null; then
    ((LOADED++)) || true
  else
    # Fallback: docker save | ctr import
    warn "  kind load failed for ${img}, trying ctr import..."
    NODE_NAME="${CLUSTER_NAME}-control-plane"
    if docker save "$img" | docker exec -i "$NODE_NAME" ctr -n k8s.io images import - 2>/dev/null; then
      ((LOADED++)) || true
    else
      warn "  Could not load ${img} into cluster"
    fi
  fi
done

success "Loaded ${LOADED} images into kind cluster '${CLUSTER_NAME}'."
echo ""
info "Done. Pods stuck in ImagePullBackOff will recover automatically."
info "To force immediate recovery: kubectl delete pods -A --field-selector=status.phase==Pending"

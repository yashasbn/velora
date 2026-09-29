#!/usr/bin/env bash
# =============================================================================
# scripts/pull-images.sh
# Downloads and caches all required Velora container images on the Docker host.
# Run this once upfront to guarantee fast, zero-failure deployments.
#
# Requires: Docker daemon configured with mirror.gcr.io in daemon.json
#   (see scripts/fix-docker.sh or run this script — it auto-configures)
#
# Usage:
#   ./scripts/pull-images.sh
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

# ---------------------------------------------------------------------------
# Auto-configure Docker Hub mirror if not already set
# ---------------------------------------------------------------------------
ensure_mirror() {
  if docker info 2>/dev/null | grep -q 'mirror.gcr.io'; then
    return 0
  fi

  info "Configuring Docker Hub mirror (mirror.gcr.io) for reliable pulls..."
  local cfg="/etc/docker/daemon.json"
  if [ -f "$cfg" ]; then
    # Merge mirror into existing config
    python3 -c "
import json, sys
with open('$cfg') as f: c = json.load(f)
c['registry-mirrors'] = ['https://mirror.gcr.io']
c.setdefault('features', {})['containerd-snapshotter'] = False
c['mtu'] = c.get('mtu', 1492)
with open('$cfg', 'w') as f: json.dump(c, f, indent=2); f.write('\n')
"
  else
    echo '{"mtu":1492,"features":{"containerd-snapshotter":false},"registry-mirrors":["https://mirror.gcr.io"]}' \
      | python3 -m json.tool > "$cfg"
  fi

  systemctl restart docker
  sleep 2
  success "Docker Hub mirror configured and daemon restarted."
}

# ---------------------------------------------------------------------------
# Ensure quay.io connectivity (handles WSL2 unroutable IPv6 / MTU issues)
# ---------------------------------------------------------------------------
ensure_quay_route() {
  if ! curl -s --connect-timeout 4 -o /dev/null -w "%{http_code}" https://quay.io/v2/ 2>/dev/null | grep -q "401"; then
    info "Detected quay.io timeout (WSL2 IPv6/MTU route bug). Auto-configuring reliable endpoint..."
    if [ "$(id -u)" -eq 0 ]; then
      sysctl -w net.ipv6.conf.all.disable_ipv6=1 -w net.ipv6.conf.default.disable_ipv6=1 >/dev/null 2>&1 || true
      grep -q 'quay.io' /etc/hosts || echo '34.233.240.53 quay.io' >> /etc/hosts
    else
      sudo bash -c "sysctl -w net.ipv6.conf.all.disable_ipv6=1 -w net.ipv6.conf.default.disable_ipv6=1 >/dev/null 2>&1 || true; grep -q 'quay.io' /etc/hosts || echo '34.233.240.53 quay.io' >> /etc/hosts" 2>/dev/null || true
    fi
  fi
}

ensure_quay_route


# ---------------------------------------------------------------------------
# Image list
# ---------------------------------------------------------------------------
IMAGES=(
  # ArgoCD components
  "quay.io/argoproj/argocd:v2.14.11"
  "ghcr.io/dexidp/dex:v2.41.1"
  "redis:7.2.4-alpine"

  # MinIO storage
  "cgr.dev/chainguard/minio:latest"
  "quay.io/minio/mc:RELEASE.2024-11-21T17-21-54Z"

  # Airflow & Database
  "apache/airflow:2.9.3"
  "postgres:16-alpine"
  "docker.io/bitnami/postgresql:latest"

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
FAILED=()

echo -e "${CYAN}================================================================${NC}"
echo -e "${CYAN}  Velora — Pre-downloading all ${TOTAL} required Docker images${NC}"
echo -e "${CYAN}================================================================${NC}"
echo ""

index=1
for img in "${IMAGES[@]}"; do
  # Skip if already present locally
  if docker image inspect "$img" &>/dev/null; then
    success "[$index/$TOTAL] $img — cached"
    ((index++)) || true
    continue
  fi

  info "[$index/$TOTAL] Pulling $img ..."
  if docker pull "$img"; then
    success "[$index/$TOTAL] $img ready."
  else
    warn "[$index/$TOTAL] FAILED $img"
    FAILED+=("$img")
  fi
  echo ""
  ((index++)) || true
done

echo ""
echo -e "${CYAN}================================================================${NC}"
if [ ${#FAILED[@]} -eq 0 ]; then
  success "All $TOTAL images pulled successfully!"
  echo ""
  echo "Next step: provision the Kind cluster with:"
  echo "  ./scripts/create-cluster.sh"
else
  warn "The following images failed to pull:"
  for f in "${FAILED[@]}"; do
    echo "  - $f"
  done
  echo ""
  fatal "Re-run: ./scripts/pull-images.sh"
fi

#!/usr/bin/env bash
# =============================================================================
# scripts/create-cluster.sh
# Provisions the 'velora' Kind cluster via Terraform and tunes node networking.
#
# Usage:
#   ./scripts/create-cluster.sh
# =============================================================================
set -euo pipefail

# Ensure Go and user Go bin paths are included in PATH
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/snap/bin:/usr/local/bin"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fatal()   { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

CLUSTER_NAME="${CLUSTER_NAME:-velora}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-$HOME/.kube/velora-config}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Pre-flight checks
info "Running pre-flight checks for cluster creation..."
for cmd in docker kind kubectl terraform; do
  command -v "$cmd" &>/dev/null || fatal "'$cmd' not found in PATH."
done
docker info &>/dev/null || fatal "Docker daemon is not running."

# 1. Provision Kind cluster via Terraform
info "Provisioning Kind cluster '${CLUSTER_NAME}' via Terraform..."
cd "$REPO_ROOT/infra/terraform"
terraform init -upgrade -input=false
terraform apply -auto-approve \
  -var="cluster_name=${CLUSTER_NAME}" \
  -var="kubeconfig_path=${KUBECONFIG_PATH}"
success "Kind cluster '${CLUSTER_NAME}' provisioned."

# 2. Configure kubectl context
info "Configuring kubectl..."
export KUBECONFIG="${KUBECONFIG_PATH}"
kubectl cluster-info
success "kubectl configured to point to '${CLUSTER_NAME}'."

# 3. Tune networking (MTU / MSS for WSL2)
info "Tuning cluster network MTU/MSS..."
for node in $(kind get nodes --name "${CLUSTER_NAME}" 2>/dev/null); do
  docker exec --privileged "$node" ip link set dev eth0 mtu 1400 2>/dev/null || true
  docker exec --privileged "$node" iptables -t mangle -A POSTROUTING -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss 1360 2>/dev/null || true
done
success "Cluster networking tuned."

echo ""
success "Cluster '${CLUSTER_NAME}' is ready!"
echo "Next step: load images into cluster nodes with:"
echo "  ./scripts/load-images.sh"

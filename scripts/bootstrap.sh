#!/usr/bin/env bash
# =============================================================================
# scripts/bootstrap.sh
# Velora one-shot local bootstrap script.
#
# Run this from inside WSL2 after completing the prerequisite tool installs.
# It will:
#   1. Provision the kind cluster via Terraform
#   2. Configure kubectl
#   3. Tune cluster network MTU/MSS for WSL2 compatibility
#   4. Install ArgoCD via Helm
#   5. Register GitHub repo and apply App-of-Apps
#
# Usage:
#   chmod +x scripts/bootstrap.sh
#   GITHUB_REPO=https://github.com/yashasbn/velora.git ./scripts/bootstrap.sh
# =============================================================================
set -euo pipefail

# Ensure Go and user Go bin paths are included in PATH (handles WSL non-login shells)
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/snap/bin:/usr/local/bin"

# ---------------------------------------------------------------------------
# Colours for readable output
# ---------------------------------------------------------------------------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fatal()   { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

# ---------------------------------------------------------------------------
# Config (override via env vars)
# ---------------------------------------------------------------------------
GITHUB_REPO="${GITHUB_REPO:-https://github.com/yashasbn/velora.git}"
CLUSTER_NAME="${CLUSTER_NAME:-velora}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-$HOME/.kube/velora-config}"
ARGOCD_NAMESPACE="argocd"
ARGOCD_CHART_VERSION="${ARGOCD_CHART_VERSION:-10.2.2}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ArgoCD app version that ships with chart 10.2.2
ARGOCD_APP_VERSION="${ARGOCD_APP_VERSION:-v2.14.11}"

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
info "Running pre-flight checks..."
for cmd in docker kind kubectl helm terraform; do
  command -v "$cmd" &>/dev/null || fatal "'$cmd' not found in PATH — see the WSL2 setup section in README.md."
done
docker info &>/dev/null || fatal "Docker daemon is not running. Start Docker Desktop."
success "All tools found and Docker is running."

# ---------------------------------------------------------------------------
# Step 1/5 — Pre-download images to host
# ---------------------------------------------------------------------------
info "Step 1/5 — Pre-downloading container images..."
"$SCRIPT_DIR/pull-images.sh"

# ---------------------------------------------------------------------------
# Step 2/5 — Provision kind cluster & tune networking
# ---------------------------------------------------------------------------
info "Step 2/5 — Provisioning Kind cluster '${CLUSTER_NAME}'..."
"$SCRIPT_DIR/create-cluster.sh"

# ---------------------------------------------------------------------------
# Step 3/5 — Load images into Kind nodes
# ---------------------------------------------------------------------------
info "Step 3/5 — Loading pre-downloaded images into Kind nodes..."
"$SCRIPT_DIR/load-images.sh"

# ---------------------------------------------------------------------------
# Step 4/4 — Install ArgoCD via Helm & Apply App-of-Apps
# ---------------------------------------------------------------------------
info "Step 4/4 — Installing ArgoCD & applying App-of-Apps..."
"$SCRIPT_DIR/install-argocd.sh"

# ---------------------------------------------------------------------------
# Credentials & Access Info
# ---------------------------------------------------------------------------
info "Gathering access credentials..."
ARGOCD_PASSWORD=$(kubectl get secret argocd-initial-admin-secret \
  -n "$ARGOCD_NAMESPACE" \
  -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "<run: kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath='{.data.password}' | base64 -d>")

echo ""
echo -e "${GREEN}================================================================${NC}"
echo -e "${GREEN}  Velora — Phase 1 Bootstrap Complete!${NC}"
echo -e "${GREEN}================================================================${NC}"
echo ""
echo "  export KUBECONFIG=${KUBECONFIG_PATH}"
echo ""
echo "  ArgoCD UI:        http://localhost:30080"
echo "  ArgoCD login:     admin / ${ARGOCD_PASSWORD}"
echo ""
echo "  Or use port-forward:"
echo "    ./scripts/port-forward.sh argocd"
echo ""
echo "  Verify:"
echo "    ./scripts/verify-phase.sh 1"
echo ""
echo -e "${CYAN}Next steps:${NC}"
echo "  1. Open http://localhost:30080 and confirm ArgoCD is healthy"
echo "  2. Replace YOUR_USERNAME in gitops/argocd/apps/*.yaml with your GitHub username"
echo "  3. Commit and push — ArgoCD will begin syncing Phase 2 workloads"
echo "  4. Run: ./scripts/verify-phase.sh 1"
echo ""
echo -e "${CYAN}Phase 2: Operator Build & Deploy${NC}"
echo "  Since the Velora Operator is custom-built and not pushed to a public registry,"
echo "  you must build the image and load it into the kind cluster whenever you start fresh:"
echo ""
echo "  cd operator"
echo "  make docker-build"
echo "  kind load docker-image ghcr.io/yashasbn/velora-operator:latest --name velora"
echo ""
echo "  Once loaded, ArgoCD will successfully start the velora-operator deployment."
echo ""

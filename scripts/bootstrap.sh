#!/usr/bin/env bash
# =============================================================================
# scripts/bootstrap.sh
# Velora one-shot local bootstrap script.
#
# Run this from inside WSL2 after completing the prerequisite tool installs.
# It will:
#   1. Provision the kind cluster via Terraform
#   2. Install ArgoCD via Helm
#   3. Register the GitHub repo with ArgoCD
#   4. Apply the App-of-Apps ApplicationCR
#   5. Print access URLs and credentials
#
# Usage:
#   chmod +x scripts/bootstrap.sh
#   GITHUB_REPO=https://github.com/yashasbn/velora.git ./scripts/bootstrap.sh
# =============================================================================
set -euo pipefail

# Ensure Go and user Go bin paths are included in PATH (handles WSL non-login shells)
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin


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

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
info "Running pre-flight checks..."
for cmd in docker kind kubectl helm terraform; do
  command -v "$cmd" &>/dev/null || fatal "'$cmd' not found in PATH — see the WSL2 setup section in the implementation plan."
done
docker info &>/dev/null || fatal "Docker daemon is not running. Start Docker Desktop."
success "All tools found and Docker is running."

# ---------------------------------------------------------------------------
# Step 1 — Terraform: provision kind cluster
# ---------------------------------------------------------------------------
info "Step 1/5 — Provisioning kind cluster '${CLUSTER_NAME}' via Terraform..."
cd "$REPO_ROOT/infra/terraform"
terraform init -upgrade -input=false
terraform apply -auto-approve \
  -var="cluster_name=${CLUSTER_NAME}" \
  -var="kubeconfig_path=${KUBECONFIG_PATH}"
success "Kind cluster '${CLUSTER_NAME}' is ready."

# ---------------------------------------------------------------------------
# Step 2 — Configure kubectl
# ---------------------------------------------------------------------------
info "Step 2/5 — Configuring kubectl..."
export KUBECONFIG="${KUBECONFIG_PATH}"
kubectl cluster-info
kubectl get nodes
success "kubectl is configured."

# ---------------------------------------------------------------------------
# Step 3 — Install ArgoCD via Helm
# ---------------------------------------------------------------------------
info "Step 3/5 — Installing ArgoCD (chart v${ARGOCD_CHART_VERSION})..."
helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
helm repo update argo

kubectl apply -f "$REPO_ROOT/gitops/argocd/install/namespace.yaml"
kubectl apply -f "$REPO_ROOT/gitops/argocd/install/repo-secret.yaml"

helm upgrade --install argocd argo/argo-cd \
  --namespace "$ARGOCD_NAMESPACE" \
  --version "$ARGOCD_CHART_VERSION" \
  --values "$REPO_ROOT/gitops/argocd/install/values.yaml" \
  --wait \
  --timeout 5m

success "ArgoCD installed."

# ---------------------------------------------------------------------------
# Step 4 — Register GitHub repo and apply App-of-Apps
# ---------------------------------------------------------------------------
info "Step 4/5 — Registering GitHub repo with ArgoCD..."

# Wait for ArgoCD server to be ready
kubectl wait --for=condition=available deployment/argocd-server \
  -n "$ARGOCD_NAMESPACE" --timeout=120s

# Patch the App-of-Apps YAML with the actual repo URL and apply
APP_OF_APPS="$REPO_ROOT/gitops/argocd/apps/velora-app-of-apps.yaml"
if grep -q "YOUR_USERNAME" "$APP_OF_APPS"; then
  warn "App-of-Apps still has YOUR_USERNAME placeholder."
  warn "Replace it with your GitHub username first, then re-run this script."
  warn "Or set GITHUB_REPO and run:"
  warn "  GITHUB_REPO=${GITHUB_REPO} ./scripts/bootstrap.sh"
else
  kubectl apply -f "$APP_OF_APPS"
  success "App-of-Apps applied — ArgoCD will now sync all workloads."
fi

# ---------------------------------------------------------------------------
# Step 5 — Print access info
# ---------------------------------------------------------------------------
info "Step 5/5 — Gathering access credentials..."
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

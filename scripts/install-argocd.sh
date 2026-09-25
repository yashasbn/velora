#!/usr/bin/env bash
# =============================================================================
# scripts/install-argocd.sh
# Installs ArgoCD via Helm, applies repo credentials, and applies the App-of-Apps.
#
# Usage:
#   ./scripts/install-argocd.sh
# =============================================================================
set -euo pipefail

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/snap/bin:/usr/local/bin"
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/velora-config}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fatal()   { echo -e "${RED}[FAIL]${NC}  $*"; exit 1; }

ARGOCD_NAMESPACE="argocd"
ARGOCD_CHART_VERSION="${ARGOCD_CHART_VERSION:-10.2.2}"
GITHUB_REPO="${GITHUB_REPO:-https://github.com/yashasbn/velora.git}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Pre-flight check
command -v kubectl &>/dev/null || fatal "'kubectl' not found in PATH."
command -v helm &>/dev/null || fatal "'helm' not found in PATH."
kubectl cluster-info &>/dev/null || fatal "Kubernetes cluster is not reachable. Ensure Kind cluster is running."

info "Installing ArgoCD (chart v${ARGOCD_CHART_VERSION})..."
helm repo add argo https://argoproj.github.io/argo-helm 2>/dev/null || true
helm repo update argo

kubectl apply -f "$REPO_ROOT/gitops/argocd/install/namespace.yaml"
kubectl apply -f "$REPO_ROOT/gitops/argocd/install/repo-secret.yaml"

MAX_HELM_ATTEMPTS=3
for attempt in $(seq 1 $MAX_HELM_ATTEMPTS); do
  info "  Helm install attempt ${attempt}/${MAX_HELM_ATTEMPTS}..."
  if helm upgrade --install argocd argo/argo-cd \
    --namespace "$ARGOCD_NAMESPACE" \
    --version "$ARGOCD_CHART_VERSION" \
    --values "$REPO_ROOT/gitops/argocd/install/values.yaml" \
    --wait \
    --timeout 10m; then
    success "ArgoCD installed on attempt ${attempt}."
    break
  fi

  if [[ $attempt -eq $MAX_HELM_ATTEMPTS ]]; then
    fatal "ArgoCD Helm install failed after ${MAX_HELM_ATTEMPTS} attempts. Check: kubectl get pods -n argocd"
  fi

  warn "  Attempt ${attempt} failed. Cleaning up before retry..."
  kubectl delete jobs -n "$ARGOCD_NAMESPACE" --all --ignore-not-found 2>/dev/null || true
  sleep 10
done

# Register GitHub repo and apply App-of-Apps
info "Registering GitHub repo with ArgoCD..."
kubectl wait --for=condition=available deployment/argocd-server \
  -n "$ARGOCD_NAMESPACE" --timeout=180s

APP_OF_APPS="$REPO_ROOT/gitops/argocd/apps/velora-app-of-apps.yaml"
if grep -q "YOUR_USERNAME" "$APP_OF_APPS"; then
  warn "App-of-Apps still has YOUR_USERNAME placeholder."
  warn "Replace it with your GitHub username first, then re-run this script."
else
  kubectl apply -f "$APP_OF_APPS"
  success "App-of-Apps applied — ArgoCD will now sync all workloads."
fi

# Credentials & Access Info
info "Gathering access credentials..."
ARGOCD_PASSWORD=$(kubectl get secret argocd-initial-admin-secret \
  -n "$ARGOCD_NAMESPACE" \
  -o jsonpath="{.data.password}" 2>/dev/null | base64 -d || echo "<run: kubectl get secret argocd-initial-admin-secret -n argocd -o jsonpath='{.data.password}' | base64 -d>")

echo ""
echo -e "${GREEN}================================================================${NC}"
echo -e "${GREEN}  Velora — ArgoCD Installation Complete!${NC}"
echo -e "${GREEN}================================================================${NC}"
echo ""
echo "  ArgoCD UI:        http://localhost:30080"
echo "  ArgoCD login:     admin / ${ARGOCD_PASSWORD}"
echo ""
echo "  Or use port-forward:"
echo "    ./scripts/port-forward.sh argocd"
echo ""

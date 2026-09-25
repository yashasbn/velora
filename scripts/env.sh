#!/usr/bin/env bash
# =============================================================================
# scripts/env.sh
# Source this file to configure your shell for Velora development.
#
# Usage (run from WSL2):
#   source scripts/env.sh
#   # or
#   . scripts/env.sh
#
# This only affects the CURRENT terminal session.
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/snap/bin"
export KUBECONFIG="$HOME/.kube/velora-config"

echo -e "\033[0;36m[velora]\033[0m  Environment loaded (this session only)"
echo "  KUBECONFIG=$KUBECONFIG"

# Quick cluster check (non-fatal)
if command -v kubectl &>/dev/null && kubectl cluster-info &>/dev/null 2>&1; then
  echo -e "  Cluster:  \033[0;32mreachable\033[0m"
else
  echo -e "  Cluster:  \033[1;33mnot running\033[0m — run ./scripts/bootstrap.sh to start"
fi

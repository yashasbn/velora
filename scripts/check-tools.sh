#!/usr/bin/env bash
# =============================================================================
# scripts/check-tools.sh
# Verifies all prerequisite tools are installed and operational.
#
# Usage:
#   ./scripts/check-tools.sh
# =============================================================================
set -uo pipefail

# Ensure standard tool paths are in PATH (WSL non-login shell compatibility)
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/snap/bin:/usr/local/bin"

# -----------------------------------------------------------------------------
# Colors & Formatting
# -----------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

echo -e "\n${BOLD}${CYAN}[velora]${NC} Checking prerequisite tools..."
echo -e "${DIM}------------------------------------------------------------${NC}"

OK_TOOLS=()
FAIL_TOOLS=()

# Helper function to check a CLI tool
# Usage: check_tool <tool_name> <command_binary> <version_cmd> [custom_hint]
check_tool() {
  local name="$1"
  local bin="$2"
  local ver_cmd="$3"
  local hint="${4:-Missing from PATH}"

  if ! command -v "$bin" &>/dev/null; then
    echo -e "  ${RED}✘${NC}  ${BOLD}${name}${NC}: ${RED}NOT OK${NC} ${DIM}(${hint})${NC}"
    FAIL_TOOLS+=("${name} (${hint})")
    return
  fi

  local version_info
  version_info=$(eval "$ver_cmd" 2>/dev/null | head -n1 | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')

  echo -e "  ${GREEN}✔${NC}  ${BOLD}${name}${NC}: ${GREEN}OK${NC} ${DIM}(${version_info})${NC}"
  OK_TOOLS+=("${name}")
}

# 1. Docker Daemon (must be installed and daemon responding)
if ! command -v docker &>/dev/null; then
  echo -e "  ${RED}✘${NC}  ${BOLD}Docker${NC}: ${RED}NOT OK${NC} ${DIM}(docker CLI not found in PATH)${NC}"
  FAIL_TOOLS+=("Docker (CLI not found in PATH)")
elif ! docker info &>/dev/null; then
  echo -e "  ${RED}✘${NC}  ${BOLD}Docker${NC}: ${RED}NOT OK${NC} ${DIM}(daemon is not running — start Docker Desktop / service)${NC}"
  FAIL_TOOLS+=("Docker (daemon not running)")
else
  docker_ver=$(docker version --format '{{.Server.Version}}' 2>/dev/null || docker --version 2>/dev/null | head -n1)
  echo -e "  ${GREEN}✔${NC}  ${BOLD}Docker${NC}: ${GREEN}OK${NC} ${DIM}(daemon running, v${docker_ver})${NC}"
  OK_TOOLS+=("Docker")
fi

# 2. kind CLI
check_tool "kind" "kind" "kind version"

# 3. kubectl CLI
check_tool "kubectl" "kubectl" "kubectl version --client 2>/dev/null | grep -i 'Client Version' || kubectl version --client -o yaml 2>/dev/null | grep 'gitVersion:' | head -n1"

# 4. Helm CLI
check_tool "helm" "helm" "helm version --short 2>/dev/null || helm version --template '{{.Version}}'"

# 5. Terraform CLI
check_tool "terraform" "terraform" "terraform version | head -n1"

# 6. Go compiler
check_tool "go" "go" "go version"

# -----------------------------------------------------------------------------
# Final Summary Message
# -----------------------------------------------------------------------------
echo -e "${DIM}------------------------------------------------------------${NC}"
TOTAL=$((${#OK_TOOLS[@]} + ${#FAIL_TOOLS[@]}))

if [[ ${#FAIL_TOOLS[@]} -eq 0 ]]; then
  echo -e "${GREEN}${BOLD}✔ ALL TOOLS OK (${#OK_TOOLS[@]}/${TOTAL})${NC}"
  echo -e "  Ready to proceed with bootstrap!"
  echo ""
  exit 0
else
  echo -e "${RED}${BOLD}✘ VERIFICATION FAILED (${#OK_TOOLS[@]}/${TOTAL} OK, ${#FAIL_TOOLS[@]} NOT OK)${NC}"
  echo -e "  ${GREEN}✔ OK:${NC}     $(IFS=', '; echo "${OK_TOOLS[*]}")"
  echo -e "  ${RED}✘ NOT OK:${NC} $(IFS=', '; echo "${FAIL_TOOLS[*]}")"
  echo -e "\nPlease install or start the failed tool(s) before continuing."
  echo ""
  exit 1
fi

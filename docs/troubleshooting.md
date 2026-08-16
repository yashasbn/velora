# Velora — Troubleshooting & Known Issues

This document captures every error encountered during the initial bootstrap of Velora, and the fix that resolved it. Use this as a reference if you hit the same issues.

---

## Teardown / Stop Everything

To cleanly destroy the cluster and reset to a clean state:

```bash
# 1. Export the Go bin path so kind is found
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin

# 2. Delete the kind cluster (removes all nodes, pods, namespaces)
kind delete cluster --name velora

# 3. Verify it's gone
kind get clusters   # should output nothing
docker ps           # velora-* containers should be gone

# 4. Remove the generated kubeconfig
rm -f ~/.kube/velora-config

# 5. Clean up Terraform state
cd /mnt/c/Projects/velora/infra/terraform
rm -f terraform.tfstate terraform.tfstate.backup .terraform.lock.hcl
```

> **Note**: The project source code in `c:\Projects\velora` is untouched. Next time, just re-run `./scripts/bootstrap.sh`.

---

## Known Issues & Fixes

---

### ❌ Issue 1: `chmod` not recognized in PowerShell

**Symptom**
```
chmod : The term 'chmod' is not recognized as the name of a cmdlet...
```

**Root Cause**  
The bootstrap script was run from **Windows PowerShell** instead of WSL2. `chmod` is a POSIX command that does not exist in PowerShell.

**Fix**  
All scripts must be run from inside the **WSL2 Ubuntu terminal**, not PowerShell or CMD.

```bash
# Open WSL2 from Windows Terminal or PowerShell:
wsl -d Ubuntu

# Navigate to the project
cd /mnt/c/Projects/velora

# Then run scripts
chmod +x scripts/*.sh
./scripts/bootstrap.sh
```

---

### ❌ Issue 2: `'kind' not found in PATH`

**Symptom**
```
[FAIL]  'kind' not found in PATH
```

**Root Cause**  
`kind` was installed via `go install sigs.k8s.io/kind@latest`, which places the binary in `$HOME/go/bin`. This directory is only added to `PATH` in **interactive login shells** (via `~/.bashrc`). When the bootstrap script is invoked non-interactively from Windows, the PATH is not sourced.

**Fix**  
The bootstrap script now explicitly exports the Go binary paths at the top:

```bash
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
```

This was added to `scripts/bootstrap.sh` so it works in any shell context.

---

### ❌ Issue 3: `'helm' not found in PATH`

**Symptom**
```
[FAIL]  'helm' not found in PATH
```

**Root Cause**  
Same root cause as Issue 2 — the script was run from a non-login shell without the full PATH.

**Fix**  
Same fix: the `export PATH` line at the top of `bootstrap.sh` covers this. Helm is installed at `/usr/local/bin/helm` via the official installer script, which is always in PATH once the script adds `/usr/local/go/bin` explicitly.

---

### ❌ Issue 4: Terraform checksum verification failure

**Symptom**
```
Error: Failed to install provider
the current package for registry.terraform.io/tehcyx/kind 0.5.1 doesn't
match any of the checksums previously recorded in the dependency lock file
```

**Root Cause**  
A `.terraform.lock.hcl` file was checked in to the repository with placeholder checksums that did not match the real provider package hashes for the Linux/AMD64 platform.

**Fix**  
Delete the lock file and let Terraform regenerate it with the correct platform checksums:

```bash
cd /mnt/c/Projects/velora/infra/terraform
rm -f .terraform.lock.hcl
terraform init -upgrade
```

Terraform will regenerate `.terraform.lock.hcl` with the correct checksums for your OS/architecture. **Commit this file** so future runs don't hit the same issue:

```bash
git add infra/terraform/.terraform.lock.hcl
git commit -m "fix(infra): add correct terraform provider lock file for linux/amd64"
```

---

### ❌ Issue 5: Redundant `provider "kind" {}` warning in Terraform

**Symptom**
```
Warning: Redundant empty provider block
  on modules/kind/main.tf line 11:
  provider "kind" {}
```

**Root Cause**  
An empty `provider "kind" {}` block was left in the child module `modules/kind/main.tf`. This was valid in older Terraform but is deprecated in Terraform >= 1.6.

**Fix**  
Removed the empty `provider "kind" {}` block from `modules/kind/main.tf`. The `required_providers` block in `terraform {}` is sufficient.

---

### ❌ Issue 6: Docker permission denied in WSL2

**Symptom**
```
permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock
```

**Root Cause**  
The user account was not in the `docker` group, so Docker API calls required `sudo`.

**Fix**
```bash
sudo usermod -aG docker $USER
sudo apt install util-linux-extra   # provides newgrp
newgrp docker                       # activate the group membership immediately
```

Verify with:
```bash
docker ps   # should work without sudo
```

---

### ❌ Issue 7: Wrong WSL distribution (docker-desktop)

**Symptom**  
Tools like `go`, `kind`, `kubectl` installed correctly but not found, or you find yourself in a minimal shell with no package manager.

**Root Cause**  
You were inside the `docker-desktop` WSL2 distribution (Docker's internal Linux VM) instead of the `Ubuntu` distribution where you installed your tools.

**Fix**  
Always explicitly open the Ubuntu distribution:

```bash
# From Windows PowerShell or Terminal:
wsl -d Ubuntu
```

Set Ubuntu as the default if needed:
```bash
wsl --set-default Ubuntu
```

---

### ❌ Issue 8: ArgoCD SSL certificate error connecting to GitHub

**Symptom**
```
Failed to load target state: tls: failed to verify certificate:
x509: certificate is valid for localhost, not github.com
```

**Root Cause**  
A network-level TLS inspection tool (e.g., Lenovo Vantage, corporate proxy, or antivirus) on the host machine intercepts HTTPS traffic and presents its own certificate. Your WSL2 shell trusts this via the system CA store, but Docker containers (and thus pods inside kind) do not trust the intercepted certificate.

**Fix — Option A: Register the repo as insecure in ArgoCD**

Apply a repository configuration secret that tells ArgoCD to skip TLS verification for this specific repository:

```bash
kubectl apply -f gitops/argocd/install/repo-secret.yaml
```

The secret (`gitops/argocd/install/repo-secret.yaml`) contains:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: velora-repo-config
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
type: Opaque
stringData:
  url: https://github.com/yashasbn/velora.git
  insecure: "true"
  type: git
```

**Fix — Option B: Disable TLS interception in Docker Desktop**  
In Docker Desktop → Settings → Resources → Proxies → disable "Manual proxy configuration" if enabled, or add `github.com` to the no-proxy list.

**Fix — Option C: Add GitHub's IP directly to CoreDNS**

If redirects are looping (error: `Maximum (20) redirects followed`), the DNS resolution inside the cluster may be affected. Add GitHub's IP directly:

```bash
# Edit CoreDNS config
kubectl -n kube-system edit configmap coredns
```

Add inside the `Corefile` data, before the final `}`:
```
hosts {
    140.82.112.4 github.com
    140.82.112.4 api.github.com
    fallthrough
}
```

Then restart CoreDNS:
```bash
kubectl -n kube-system rollout restart deployment coredns
```

---

## Environment Architecture (for reference)

```
Windows
│
├── Docker Desktop
│      │
│      └── docker-desktop (WSL2 distro)
│              └── Docker Engine ◄─────────────────────────────┐
│                                                               │
└── Ubuntu (WSL2 distro)                                       │
       ├── Go 1.22.5   (/usr/local/go/bin)                     │
       ├── kubectl      (/snap/bin/kubectl)                     │
       ├── kind         ($HOME/go/bin/kind)  ─── creates ──► Docker containers
       ├── Terraform    (/snap/bin/terraform)                   │
       ├── Helm         (/usr/local/bin/helm)                   │
       └── /mnt/c/Projects/velora  (project root)
```

> **Key rule**: All scripts (`bootstrap.sh`, `port-forward.sh`, `verify-phase.sh`) must be run from the **Ubuntu WSL2** shell, not from Windows PowerShell.

---

### ❌ Issue 9: Terraform control plane node error

**Symptom**
```
Error: could not locate any control plane nodes for cluster named 'velora'. Use the --name option to select a different cluster
```

**Root Cause**  
Terraform's local state (`terraform.tfstate`) believes the `velora` cluster exists, but the Docker containers for the cluster were deleted (e.g., via a Docker prune, reboot, or manual deletion). 

**Fix**  
Delete the local Terraform state to force a fresh cluster creation:
```bash
cd /mnt/c/Projects/velora/infra/terraform
rm -f terraform.tfstate terraform.tfstate.backup
```
Then re-run `./scripts/bootstrap.sh`.

---

### ❌ Issue 10: Helm timeout during ArgoCD install

**Symptom**
```
Error: failed pre-install: 1 error occurred:
        * timed out waiting for the condition
```

**Root Cause**  
Helm uses a 5-minute timeout (`--timeout 5m`) by default for the ArgoCD installation. When installing on a fresh cluster with a slower internet connection, downloading the large ArgoCD container images from `quay.io` can take longer than 5 minutes, causing Helm's pre-install hook (`argocd-redis-secret-init` job) to time out while stuck in `ContainerCreating` (Pulling image).

**Fix**  
The image pull actually continues in the background. Wait a few more minutes for the pull to finish, and simply re-run `./scripts/bootstrap.sh`. Since the image will now be cached on the cluster node, the installation will proceed instantly.

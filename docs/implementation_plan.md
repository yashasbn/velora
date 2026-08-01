# Velora — Implementation Plan & WSL2 Setup Guide

This document describes the implementation phases of Velora and details how to set up the WSL2 environment.

---

## Windows/WSL2 Setup (Prerequisite)

This plan assumes a WSL2 Ubuntu environment, not native Windows/PowerShell. Several tools in this stack (bash scripts, kubebuilder's Makefile workflow, Ansible, envtest) require a POSIX environment.

### 1. Install WSL2
Open PowerShell as Administrator and run:
```powershell
wsl --install
```
Reboot when prompted. This installs Ubuntu by default.

### 2. Docker Desktop — Enable WSL2 Backend
In Docker Desktop settings: 
- Settings → General → Check **"Use the WSL 2 based engine"**
- Settings → Resources → WSL Integration → Enable integration with your Ubuntu distro.

### 3. Install Tooling Inside WSL2
Run these commands inside your WSL2 terminal to install the correct packages:

#### Go 1.22+
```bash
curl -LO https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

#### kubectl
```bash
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

#### Helm
```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

#### kind
```bash
# For AMD64 / x86_64
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.23.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```

#### Terraform
```bash
wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com/gpg $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install terraform
```

#### kubebuilder (v4)
```bash
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/v4.0.0/linux/amd64"
chmod +x kubebuilder && sudo mv kubebuilder /usr/local/bin/
```

### 4. Editor Setup
Install the VS Code **"WSL"** extension on Windows.
Open your project inside WSL2:
```bash
cd /mnt/c/Projects/velora
code .
```
This keeps line endings as `LF` (not Windows `CRLF`), preventing scripts from breaking.

---

## Verification Checklist

Run these commands inside WSL2 to verify everything is correctly configured before running the bootstrap script:
```bash
docker ps          # Should output container list (Docker daemon integration working)
go version         # Should be go1.22+
kubectl version --client
kind version
terraform version
kubebuilder version
```

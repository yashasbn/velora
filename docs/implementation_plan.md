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

If you encounter a permission denied error when running `docker ps` inside Ubuntu:
```bash
sudo usermod -aG docker $USER
sudo apt install util-linux-extra
newgrp docker
```

### 3. Install Tooling Inside WSL2
Run these commands inside your WSL2 Ubuntu terminal:

#### Go 1.22+
```bash
curl -LO https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

#### kubectl
Install via Snap:
```bash
sudo snap install kubectl --classic
```

#### Helm
```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

#### kind
Install via Go:
```bash
go install sigs.k8s.io/kind@latest
```

#### Terraform
Install via Snap:
```bash
sudo snap install terraform --classic
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

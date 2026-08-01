terraform {
  required_version = ">= 1.6"
  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "0.5.1"
    }
  }
}

# ---------------------------------------------------------------------------
# Target selection
# Set var.target = "kind" (default) for local development.
# Set var.target = "gke"  for the optional cloud path (Phase 6).
# ---------------------------------------------------------------------------
variable "target" {
  description = "Deployment target: 'kind' (local) or 'gke' (cloud — Phase 6)"
  type        = string
  default     = "kind"

  validation {
    condition     = contains(["kind", "gke"], var.target)
    error_message = "target must be 'kind' or 'gke'."
  }
}

# ---------------------------------------------------------------------------
# kind (local) cluster
# ---------------------------------------------------------------------------
module "kind" {
  source = "./modules/kind"
  count  = var.target == "kind" ? 1 : 0

  cluster_name       = var.cluster_name
  kubernetes_version = var.kubernetes_version
  kubeconfig_path    = var.kubeconfig_path
  worker_count       = var.worker_count
}

# ---------------------------------------------------------------------------
# GKE (cloud) cluster — stub, implemented in Phase 6
# ---------------------------------------------------------------------------
# module "gke" {
#   source = "./modules/gke"
#   count  = var.target == "gke" ? 1 : 0
#   ...
# }

# ---------------------------------------------------------------------------
# Shared variables (passed to whichever module is active)
# ---------------------------------------------------------------------------
variable "cluster_name" {
  type    = string
  default = "velora"
}

variable "kubernetes_version" {
  type    = string
  default = "v1.30.2"
}

variable "kubeconfig_path" {
  type    = string
  default = "~/.kube/velora-config"
}

variable "worker_count" {
  type    = number
  default = 2
}

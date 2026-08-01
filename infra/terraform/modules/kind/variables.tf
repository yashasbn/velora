variable "cluster_name" {
  description = "Name of the kind cluster"
  type        = string
  default     = "velora"
}

variable "kubernetes_version" {
  description = "Kubernetes version to use (must match a kindest/node image tag)"
  type        = string
  default     = "v1.30.2"
}

variable "kubeconfig_path" {
  description = "Path where the kubeconfig file will be written"
  type        = string
  default     = "~/.kube/velora-config"
}

variable "worker_count" {
  description = "Number of worker nodes"
  type        = number
  default     = 2
}

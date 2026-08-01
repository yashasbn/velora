output "cluster_name" {
  description = "Name of the active cluster"
  value       = var.target == "kind" ? module.kind[0].cluster_name : "gke-cluster"
}

output "kubeconfig_path" {
  description = "Path to the kubeconfig file"
  value       = var.target == "kind" ? module.kind[0].kubeconfig_path : "~/.kube/gke-config"
}

output "cluster_endpoint" {
  description = "Kubernetes API server endpoint"
  value       = var.target == "kind" ? module.kind[0].cluster_endpoint : ""
  sensitive   = true
}

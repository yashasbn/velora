output "cluster_name" {
  description = "Name of the provisioned kind cluster"
  value       = kind_cluster.velora.name
}

output "kubeconfig_path" {
  description = "Absolute path to the kubeconfig file for this cluster"
  value       = pathexpand(var.kubeconfig_path)
}

output "cluster_endpoint" {
  description = "Kubernetes API server endpoint"
  value       = kind_cluster.velora.endpoint
}

output "client_certificate" {
  description = "Client certificate for authenticating to the cluster"
  value       = kind_cluster.velora.client_certificate
  sensitive   = true
}

output "client_key" {
  description = "Client key for authenticating to the cluster"
  value       = kind_cluster.velora.client_key
  sensitive   = true
}

output "cluster_ca_certificate" {
  description = "Cluster CA certificate"
  value       = kind_cluster.velora.cluster_ca_certificate
  sensitive   = true
}

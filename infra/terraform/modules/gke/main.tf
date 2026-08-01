# GKE Autopilot Module — Phase 6 (Cloud-Optional Target)
# Ref: https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_cluster

terraform {
  required_version = ">= 1.6"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

variable "cluster_name" {
  type    = string
  default = "velora-gke"
}

variable "region" {
  type    = string
  default = "us-central1"
}

# Define the private GKE Autopilot Cluster.
resource "google_container_cluster" "velora_gke" {
  name     = var.cluster_name
  location = var.region

  # Enable Autopilot
  enable_autopilot = true

  # Release channel for automatic master/node updates
  release_channel {
    channel = "REGULAR"
  }

  ip_allocation_policy {
    # Allocates VPC native IP blocks automatically
  }

  # Configures private cluster settings for security
  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false # Keep endpoint public for easy local kubectl access
    master_global_access_config {
      enabled = true
    }
  }
}

output "cluster_name" {
  value = google_container_cluster.velora_gke.name
}

output "cluster_endpoint" {
  value = google_container_cluster.velora_gke.endpoint
}

output "cluster_ca_certificate" {
  value     = google_container_cluster.velora_gke.master_auth[0].cluster_ca_certificate
  sensitive = true
}

terraform {
  required_version = ">= 1.6"
  required_providers {
    kind = {
      source  = "tehcyx/kind"
      version = "0.5.1"
    }
  }
}

provider "kind" {}

locals {
  # Build the list of worker nodes dynamically based on var.worker_count
  worker_nodes = [for i in range(var.worker_count) : { role = "worker" }]
}

resource "kind_cluster" "velora" {
  name            = var.cluster_name
  node_image      = "kindest/node:${var.kubernetes_version}"
  wait_for_ready  = true
  kubeconfig_path = pathexpand(var.kubeconfig_path)

  kind_config {
    kind        = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"

    # Control plane node with host port mappings so services are
    # reachable from the Windows host (or the WSL2 distro).
    node {
      role = "control-plane"

      # ArgoCD UI
      extra_port_mappings {
        container_port = 30080
        host_port      = 30080
        protocol       = "TCP"
      }

      # Airflow webserver
      extra_port_mappings {
        container_port = 30081
        host_port      = 30081
        protocol       = "TCP"
      }

      # MinIO S3 API
      extra_port_mappings {
        container_port = 30090
        host_port      = 30090
        protocol       = "TCP"
      }

      # MinIO console
      extra_port_mappings {
        container_port = 30091
        host_port      = 30091
        protocol       = "TCP"
      }

      # Grafana UI
      extra_port_mappings {
        container_port = 30300
        host_port      = 30300
        protocol       = "TCP"
      }

      kubeadm_config_patches = [
        <<-YAML
          kind: InitConfiguration
          nodeRegistration:
            kubeletExtraArgs:
              node-labels: "ingress-ready=true"
        YAML
      ]
    }

    # Worker nodes (count driven by var.worker_count)
    dynamic "node" {
      for_each = range(var.worker_count)
      content {
        role = "worker"
      }
    }
  }
}

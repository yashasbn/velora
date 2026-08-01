# GKE Module (Phase 6 — Optional Cloud Target)

This module is a stub for extending Velora to GKE Autopilot.
It is intentionally left unimplemented in Phase 1.

To activate it, set `var.target = "gke"` in the root module
and implement this module using `google_container_cluster`.

## Future implementation notes

- Use `google_container_cluster` with `enable_autopilot = true`
- Workload Identity for pod-level GCP permissions (no static SA keys)
- Private cluster with authorized networks
- Node pools: spot instances for worker nodes (cost optimisation)
- Output the cluster CA cert and endpoint for the kubernetes provider

See [GKE Autopilot docs](https://cloud.google.com/kubernetes-engine/docs/concepts/autopilot-overview)

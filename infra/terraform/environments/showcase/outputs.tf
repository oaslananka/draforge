# infra/terraform/environments/showcase/outputs.tf

output "kubernetes_cluster_endpoint" {
  value       = digitalocean_kubernetes_cluster.draforge_cluster.endpoint
  description = "The endpoint for the Kubernetes API server."
}

output "kubernetes_cluster_urn" {
  value       = digitalocean_kubernetes_cluster.draforge_cluster.urn
  description = "The URN of the Kubernetes cluster."
}

output "container_registry_endpoint" {
  value       = digitalocean_container_registry.draforge_registry.endpoint
  description = "The endpoint for the container registry."
}

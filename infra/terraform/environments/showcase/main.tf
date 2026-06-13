# infra/terraform/environments/showcase/main.tf

terraform {
  required_version = ">= 1.0.0"
  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.30"
    }
  }
  backend "local" {
    path = "terraform.tfstate"
  }
}

provider "digitalocean" {
  # Token is automatically loaded from the DIGITALOCEAN_TOKEN environment variable
}

# 1. Dedicated VPC
resource "digitalocean_vpc" "draforge_vpc" {
  name     = "draforge-vpc-showcase"
  region   = var.region
  ip_range = "10.120.0.0/16"
}

# 2. DigitalOcean Kubernetes Service (DOKS)
resource "digitalocean_kubernetes_cluster" "draforge_cluster" {
  name     = "draforge-cluster"
  region   = var.region
  version  = var.kubernetes_version
  vpc_uuid = digitalocean_vpc.draforge_vpc.id

  ha            = false
  auto_upgrade  = false
  tags          = var.tags

  node_pool {
    name       = "draforge-workers"
    size       = var.node_size
    node_count = var.node_count
    tags       = var.tags
  }
}

# 3. DigitalOcean Container Registry (DOCR)
# Note: Since the registry 'draforge' was pre-created, we will import it into Terraform state.
resource "digitalocean_container_registry" "draforge_registry" {
  name                   = var.registry_name
  subscription_tier_slug = "starter"
  region                 = "tor1" # Pre-created region
}

# 4. Dedicated Project
resource "digitalocean_project" "draforge_project" {
  name        = var.project_name
  description = "Explain, simulate, test, and visualize Kubernetes Dynamic Resource Allocation."
  purpose     = "Developer Tools"
  environment = "development"
  resources = [
    digitalocean_kubernetes_cluster.draforge_cluster.urn
  ]
}

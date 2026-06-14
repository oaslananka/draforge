# infra/terraform/environments/showcase/variables.tf

variable "project_name" {
  type        = string
  description = "The name of the project"
  default     = "draforge"
}

variable "environment" {
  type        = string
  description = "The deployment environment"
  default     = "showcase"
}

variable "region" {
  type        = string
  description = "The DigitalOcean region"
  default     = "fra1"
}

variable "kubernetes_version" {
  type        = string
  description = "The target Kubernetes version"
  default     = "1.36.0-do.1"
}

variable "node_size" {
  type        = string
  description = "The size of the DOKS worker nodes"
  default     = "s-4vcpu-8gb"
  validation {
    condition     = !can(regex(".*gpu.*|.*g-.*", var.node_size))
    error_message = "GPU nodes are strictly prohibited."
  }
}

variable "node_count" {
  type        = number
  description = "The number of worker nodes"
  default     = 2
  validation {
    condition     = var.node_count >= 1 && var.node_count <= 2
    error_message = "The node count must be between 1 and 2."
  }
}

variable "registry_name" {
  type        = string
  description = "The name of the container registry"
  default     = "draforge"
}

variable "enable_public_dashboard" {
  type        = bool
  description = "Whether to enable the public dashboard"
  default     = true
}

variable "domain_name" {
  type        = string
  description = "Custom domain name (optional)"
  default     = ""
}

variable "allowed_admin_cidrs" {
  type        = list(string)
  description = "List of CIDRs allowed for administrative access"
  default     = ["0.0.0.0/0"]
}

variable "tags" {
  type        = list(string)
  description = "Tags to apply to resources"
  default     = ["project:draforge", "managed-by:terraform", "owner:oaslananka", "environment:showcase"]
}

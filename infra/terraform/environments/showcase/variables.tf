# infra/terraform/environments/showcase/variables.tf

variable "project_name" {
  type        = string
  description = "DigitalOcean project name used only for resource grouping; changing it does not create billable compute resources."
  default     = "draforge"
}

variable "region" {
  type        = string
  description = "DigitalOcean region for the showcase VPC and DOKS cluster; region choice affects availability, latency, and provider pricing."
  default     = "fra1"
}

variable "kubernetes_version" {
  type        = string
  description = "DOKS Kubernetes version for the optional showcase; it must remain compatible with the supported DRA API and auto-upgrade is disabled."
  default     = "1.36.0-do.1"
}

variable "node_size" {
  type        = string
  description = "DOKS worker size and the primary showcase compute-cost driver; GPU node sizes are prohibited by policy."
  default     = "s-4vcpu-8gb"
  validation {
    condition     = !can(regex(".*gpu.*|.*g-.*", var.node_size))
    error_message = "GPU nodes are strictly prohibited."
  }
}

variable "node_count" {
  type        = number
  description = "Number of billable DOKS workers; cost scales with this value and policy limits the showcase to one or two nodes."
  default     = 2
  validation {
    condition     = var.node_count >= 1 && var.node_count <= 2
    error_message = "The node count must be between 1 and 2."
  }
}

variable "registry_name" {
  type        = string
  description = "DigitalOcean Container Registry name used by the optional showcase; the configured registry subscription may incur provider charges."
  default     = "draforge"
}

variable "tags" {
  type        = list(string)
  description = "Resource inventory and cost-attribution tags; tags are metadata and do not provide an authorization or network-security boundary."
  default     = ["project:draforge", "managed-by:terraform", "owner:oaslananka", "environment:showcase"]
}

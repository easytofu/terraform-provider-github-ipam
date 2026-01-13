# Configure the Git IPAM Provider
provider "gitipam" {
  # GitHub Personal Access Token or App Installation Token
  # Can also be set via GITHUB_TOKEN environment variable
  token = var.github_token

  # GitHub repository owner (user or organization)
  owner = "my-org"

  # Repository containing IPAM data files
  repository = "ipam-config"

  # Path to pools.yaml (read-only pool definitions)
  pools_file = "config/pools.yaml"

  # Path to allocations.yaml (read-write allocation state)
  allocations_file = "config/allocations.yaml"

  # Git branch (default: main)
  branch = "main"

  # Retry configuration for optimistic concurrency control
  max_retries   = 10  # Maximum retry attempts on conflict
  base_delay_ms = 200 # Base delay for exponential backoff
}

variable "github_token" {
  description = "GitHub token for IPAM operations"
  type        = string
  sensitive   = true
}

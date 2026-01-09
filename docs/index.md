---
page_title: "Provider: github-ipam"
description: |-
  Git-backed IPAM provider using GitHub API for IP address management with optimistic concurrency control.
---

# GitHub IPAM Provider

The GitHub IPAM provider enables GitOps-native IP address management by storing CIDR allocations in a GitHub repository. It uses GitHub's API for optimistic concurrency control, ensuring safe concurrent allocations without race conditions.

## Features

- **Dual-file architecture**: Pool definitions in `pools.yaml` (read-only, managed via PR) and allocations in `allocations.json` (read-write with OCC)
- **Two allocation modes**:
  - `pool_id`: Allocate from pools defined in pools.yaml
  - `parent_cidr`: Sub-allocate from existing CIDR blocks (hierarchical allocation)
- **Optimistic concurrency control**: Uses GitHub API SHA verification to prevent race conditions
- **Exponential backoff with jitter**: Automatic retry on conflicts (configurable)
- **CIDR alignment**: Automatically aligns allocations to proper network boundaries

## Example Usage

```hcl
terraform {
  required_providers {
    github-ipam = {
      source  = "easytofu/github-ipam"
      version = "~> 0.0.1"
    }
  }
}

provider "github-ipam" {
  token            = var.github_token
  owner            = "my-org"
  repository       = "network-config"
  branch           = "main"
  pools_file       = "network/pools.yaml"
  allocations_file = "network/allocations.json"
}

# Allocate a /16 VPC from a pool
resource "github-ipam_allocation" "vpc" {
  pool_id   = "production"
  cidr_mask = 16
  name      = "vpc-prod-us-east-1"

  metadata = {
    environment = "production"
    region      = "us-east-1"
  }
}

# Sub-allocate a /24 subnet from the VPC
resource "github-ipam_allocation" "subnet" {
  parent_cidr = github-ipam_allocation.vpc.cidr
  cidr_mask   = 24
  name        = "subnet-public"

  metadata = {
    subnet_type = "public"
  }
}
```

## Pool Configuration (pools.yaml)

The provider reads pool definitions from a YAML file in your repository:

```yaml
pools:
  production:
    description: "Production network pool"
    cidr:
      - "10.0.0.0/8"
    metadata:
      environment: production

  development:
    description: "Development network pool"
    cidr:
      - "172.16.0.0/12"
    metadata:
      environment: development
```

## Allocations State (allocations.json)

The provider manages allocation state in a JSON file:

```json
{
  "version": 1,
  "allocations": {
    "production": [
      {
        "id": "abc123",
        "cidr": "10.0.0.0/16",
        "name": "vpc-prod-us-east-1",
        "created_at": "2024-01-15T10:30:00Z"
      }
    ]
  }
}
```

## Authentication

The provider requires a GitHub token with repository read/write access. You can provide it via:

1. The `token` provider attribute
2. The `GITHUB_TOKEN` environment variable

For GitHub Actions, use `${{ secrets.GITHUB_TOKEN }}` or a Personal Access Token with `repo` scope.

{{ .SchemaMarkdown | trimspace }}

# Terraform Provider for Git-backed IPAM

[![Tests](https://github.com/easytofu/terraform-provider-ipam-github/actions/workflows/test.yml/badge.svg)](https://github.com/easytofu/terraform-provider-ipam-github/actions/workflows/test.yml)

A Terraform/OpenTofu provider for Git-backed IP Address Management (IPAM) using GitHub as the storage backend with optimistic concurrency control.

## Features

- **Git-native**: All IP allocations stored in a GitHub repository
- **Optimistic Concurrency Control**: Safe concurrent allocations using GitHub API SHA verification
- **Dual-file Architecture**:
  - `pools.yaml` - Read-only pool definitions (managed via PR)
  - `allocations.yaml` - Read-write allocation state
- **Two Allocation Modes**:
  - **pool_id**: Allocate from pre-defined pools in pools.yaml
  - **parent_cidr**: Sub-allocate from existing allocations (e.g., subnets within VPCs)
- **Cost Effective**: Alternative to AWS VPC IPAM (~$233K/year savings at 100K+ IPs)

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0 or [OpenTofu](https://opentofu.org/) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.21 (for building from source)
- GitHub repository with pools.yaml configured

## Installation

### From OpenTofu Registry

```hcl
terraform {
  required_providers {
    gitipam = {
      source  = "registry.opentofu.org/easytofu/gitipam"
      version = "~> 0.1"
    }
  }
}
```

### Building from Source

```bash
git clone https://github.com/easytofu/terraform-provider-ipam-github.git
cd terraform-provider-ipam-github
make install
```

## Usage

### Provider Configuration

```hcl
provider "gitipam" {
  token             = var.github_token
  owner             = "my-org"
  repository        = "ipam-config"
  pools_file        = "config/pools.yaml"
  allocations_file  = "config/allocations.yaml"
  branch            = "main"
}
```

### pools.yaml Example

```yaml
pools:
  production-us-east:
    cidr: ["10.0.0.0/14"]
    description: "Production US East"
    metadata:
      environment: production
      region: us-east-1

  development:
    cidr: ["10.128.0.0/16"]
    description: "Development Environment"
    metadata:
      environment: development
```

### Allocating a VPC CIDR (Mode 1: pool_id)

```hcl
resource "gitipam_allocation" "vpc" {
  pool_id   = "production-us-east"
  cidr_mask = 16
  name      = "main-vpc"

  metadata = {
    purpose = "vpc"
    team    = "platform"
  }
}

resource "aws_vpc" "main" {
  cidr_block = gitipam_allocation.vpc.cidr
}
```

### Allocating Subnets (Mode 2: parent_cidr)

```hcl
resource "gitipam_allocation" "subnet_public" {
  parent_cidr = gitipam_allocation.vpc.cidr
  cidr_mask   = 24
  name        = "public-subnet-az1"

  metadata = {
    tier = "public"
    az   = "us-east-1a"
  }
}

resource "aws_subnet" "public" {
  vpc_id     = aws_vpc.main.id
  cidr_block = gitipam_allocation.subnet_public.cidr
}
```

## Documentation

Full documentation is available in the [docs](./docs/) directory and on the [OpenTofu Registry](https://registry.opentofu.org/providers/easytofu/gitipam/latest/docs).

## Development

```bash
# Run tests
make test

# Run acceptance tests (requires GitHub token)
export GITHUB_TOKEN="your-token"
make testacc

# Build and install locally
make install

# Run linting
make lint
```

## License

[MPL-2.0](LICENSE)

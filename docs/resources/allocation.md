---
page_title: "github-ipam_allocation Resource - github-ipam"
subcategory: ""
description: |-
  Manages a CIDR allocation in the GitHub-backed IPAM system.
---

# github-ipam_allocation (Resource)

Manages a CIDR allocation in the GitHub-backed IPAM system. Supports two allocation modes:

1. **Pool allocation** (`pool_id`): Allocate from pools defined in `pools.yaml`
2. **Parent CIDR allocation** (`parent_cidr`): Sub-allocate from an existing CIDR block

The provider automatically finds the next available CIDR block that fits the requested prefix length, handles conflicts with exponential backoff, and ensures proper network boundary alignment.

## Example Usage

### Pool Allocation (Mode 1)

Allocate a CIDR block directly from a pool defined in `pools.yaml`:

```hcl
resource "github-ipam_allocation" "vpc" {
  pool_id   = "production"
  cidr_mask = 16
  name      = "vpc-prod-us-east-1"

  metadata = {
    environment = "production"
    region      = "us-east-1"
    purpose     = "vpc"
  }
}
```

### Parent CIDR Allocation (Mode 2)

Sub-allocate from an existing CIDR block (hierarchical allocation):

```hcl
resource "github-ipam_allocation" "subnet_public" {
  parent_cidr = github-ipam_allocation.vpc.cidr
  cidr_mask   = 24
  name        = "subnet-public-a"

  metadata = {
    subnet_type       = "public"
    availability_zone = "us-east-1a"
  }
}

resource "github-ipam_allocation" "subnet_private" {
  parent_cidr = github-ipam_allocation.vpc.cidr
  cidr_mask   = 20
  name        = "subnet-private-a"

  metadata = {
    subnet_type       = "private"
    availability_zone = "us-east-1a"
  }
}
```

### Complete VPC Example

```hcl
# Allocate VPC CIDR from pool
resource "github-ipam_allocation" "vpc" {
  pool_id   = "aws-us-east-1"
  cidr_mask = 16
  name      = "vpc-main"
}

# Allocate subnets from VPC CIDR
resource "github-ipam_allocation" "public" {
  parent_cidr = github-ipam_allocation.vpc.cidr
  cidr_mask   = 24
  name        = "subnet-public"
}

resource "github-ipam_allocation" "compute" {
  parent_cidr = github-ipam_allocation.vpc.cidr
  cidr_mask   = 20
  name        = "subnet-compute"
}

resource "github-ipam_allocation" "data" {
  parent_cidr = github-ipam_allocation.vpc.cidr
  cidr_mask   = 22
  name        = "subnet-data"
}

# Use allocations in AWS resources
resource "aws_vpc" "main" {
  cidr_block = github-ipam_allocation.vpc.cidr
}

resource "aws_subnet" "public" {
  vpc_id     = aws_vpc.main.id
  cidr_block = github-ipam_allocation.public.cidr
}
```

{{ .SchemaMarkdown | trimspace }}

## Import

Allocations can be imported using the allocation ID:

```shell
tofu import github-ipam_allocation.example <allocation-id>
```

The allocation ID can be found in the `allocations.json` file in your repository.

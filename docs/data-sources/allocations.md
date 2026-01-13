---
page_title: "github-ipam_allocations Data Source - github-ipam"
subcategory: ""
description: |-
  Lists allocations, optionally filtered by pool_id or parent_cidr.
---

# github-ipam_allocations (Data Source)

Lists allocations from `allocations.yaml`, optionally filtered by `pool_id` or `parent_cidr`. Use this data source to discover existing allocations and avoid conflicts.

## Example Usage

### List All Allocations

```hcl
data "github-ipam_allocations" "all" {}

output "all_allocations" {
  value = data.github-ipam_allocations.all.allocations
}
```

### Filter by Pool ID

```hcl
data "github-ipam_allocations" "production" {
  pool_id = "production"
}

output "production_allocations" {
  value = data.github-ipam_allocations.production.allocations
}
```

### Filter by Parent CIDR

```hcl
data "github-ipam_allocations" "vpc_subnets" {
  parent_cidr = "10.0.0.0/16"
}

output "vpc_subnets" {
  value = data.github-ipam_allocations.vpc_subnets.allocations
}
```

{{ .SchemaMarkdown | trimspace }}

---
page_title: "github-ipam_next_available Data Source - github-ipam"
subcategory: ""
description: |-
  Preview the next available CIDR block without allocating it.
---

# github-ipam_next_available (Data Source)

Preview the next available CIDR block without allocating it. This data source is useful for planning and dry-run scenarios.

~> **Note:** The returned CIDR is not reserved and may be claimed by another allocation before you use it. Use the `github-ipam_allocation` resource to actually allocate the CIDR.

## Example Usage

### Preview Next Available in Pool

```hcl
data "github-ipam_next_available" "vpc" {
  pool_id   = "production"
  cidr_mask = 16
}

output "next_vpc_cidr" {
  value = data.github-ipam_next_available.vpc.cidr
}
```

### Preview Next Available in Parent CIDR

```hcl
data "github-ipam_next_available" "subnet" {
  parent_cidr = "10.0.0.0/16"
  cidr_mask   = 24
}

output "next_subnet_cidr" {
  value = data.github-ipam_next_available.subnet.cidr
}
```

### Planning Example

```hcl
# Preview what would be allocated
data "github-ipam_next_available" "preview" {
  pool_id   = "production"
  cidr_mask = 16
}

output "planned_cidr" {
  description = "The CIDR that will be allocated (subject to change)"
  value       = data.github-ipam_next_available.preview.cidr
}

# Actually allocate when ready
resource "github-ipam_allocation" "vpc" {
  pool_id   = "production"
  cidr_mask = 16
  name      = "vpc-prod"
}
```

{{ .SchemaMarkdown | trimspace }}

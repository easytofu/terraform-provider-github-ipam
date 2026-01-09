---
page_title: "github-ipam_pools Data Source - github-ipam"
subcategory: ""
description: |-
  Lists all IP pools defined in pools.yaml.
---

# github-ipam_pools (Data Source)

Lists all IP pools defined in `pools.yaml`. Use this data source to discover available pools for allocation.

## Example Usage

```hcl
data "github-ipam_pools" "all" {}

output "available_pools" {
  value = data.github-ipam_pools.all.pools
}

# Use pool information dynamically
resource "github-ipam_allocation" "vpc" {
  for_each = { for pool in data.github-ipam_pools.all.pools : pool.pool_id => pool }

  pool_id   = each.key
  cidr_mask = 16
  name      = "vpc-${each.key}"
}
```

{{ .SchemaMarkdown | trimspace }}

---
page_title: "github-ipam_pool Data Source - github-ipam"
subcategory: ""
description: |-
  Retrieves details about a specific IP pool.
---

# github-ipam_pool (Data Source)

Retrieves details about a specific IP pool defined in `pools.yaml`. Use this data source to get pool metadata and available CIDR ranges.

## Example Usage

```hcl
data "github-ipam_pool" "production" {
  pool_id = "production"
}

output "pool_cidrs" {
  value = data.github-ipam_pool.production.cidrs
}

output "pool_description" {
  value = data.github-ipam_pool.production.description
}
```

{{ .SchemaMarkdown | trimspace }}

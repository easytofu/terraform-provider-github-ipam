// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package datasources

import (
	"context"
	"fmt"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/easytofu/terraform-provider-ipam-github/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var _ datasource.DataSource = &NextAvailableDataSource{}
var _ datasource.DataSourceWithConfigure = &NextAvailableDataSource{}

// NextAvailableDataSource defines the data source implementation.
type NextAvailableDataSource struct {
	client *client.GitHubClient
}

// NextAvailableDataSourceModel describes the data source data model.
type NextAvailableDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	PoolID     types.String `tfsdk:"pool_id"`
	ParentCIDR types.String `tfsdk:"parent_cidr"`
	CIDRMask   types.Int64  `tfsdk:"cidr_mask"`
	CIDR       types.String `tfsdk:"cidr"`
}

// NewNextAvailableDataSource creates a new data source.
func NewNextAvailableDataSource() datasource.DataSource {
	return &NextAvailableDataSource{}
}

func (d *NextAvailableDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_next_available"
}

func (d *NextAvailableDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Preview the next available CIDR block without allocating it. " +
			"This data source is useful for planning and dry-run scenarios.",
		MarkdownDescription: `Preview the next available CIDR block without allocating it.

This data source is useful for planning and dry-run scenarios. Note that the returned CIDR
is not reserved and may be claimed by another allocation before you use it.

**Important:** Either ` + "`pool_id`" + ` or ` + "`parent_cidr`" + ` must be specified, but not both.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"pool_id": schema.StringAttribute{
				Description: "Pool ID to allocate from (Mode 1). Mutually exclusive with parent_cidr.",
				Optional:    true,
			},
			"parent_cidr": schema.StringAttribute{
				Description: "Parent CIDR to sub-allocate from (Mode 2). Must be an existing allocation.",
				Optional:    true,
			},
			"cidr_mask": schema.Int64Attribute{
				Description: "The prefix length for the allocation (e.g., 24 for /24).",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.Between(1, 128),
				},
			},
			"cidr": schema.StringAttribute{
				Description: "The next available CIDR block that would be allocated.",
				Computed:    true,
			},
		},
	}
}

func (d *NextAvailableDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.GitHubClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.GitHubClient, got: %T. Please report this issue.", req.ProviderData),
		)
		return
	}

	d.client = client
}

func (d *NextAvailableDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NextAvailableDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate mutual exclusivity
	hasPoolID := !data.PoolID.IsNull() && !data.PoolID.IsUnknown()
	hasParentCIDR := !data.ParentCIDR.IsNull() && !data.ParentCIDR.IsUnknown()

	if !hasPoolID && !hasParentCIDR {
		resp.Diagnostics.AddAttributeError(
			path.Root("pool_id"),
			"Missing Required Configuration",
			"Either pool_id or parent_cidr must be specified.",
		)
		return
	}

	if hasPoolID && hasParentCIDR {
		resp.Diagnostics.AddAttributeError(
			path.Root("pool_id"),
			"Conflicting Configuration",
			"Only one of pool_id or parent_cidr can be specified, not both.",
		)
		return
	}

	allocator := ipam.NewAllocator()
	prefixLen := int(data.CIDRMask.ValueInt64())

	var cidr string
	var err error

	if hasPoolID {
		// Mode 1: Pool allocation
		poolID := data.PoolID.ValueString()

		// Get pools
		poolsConfig, poolErr := d.client.GetPools(ctx)
		if poolErr != nil {
			resp.Diagnostics.AddError(
				"Failed to Read Pools",
				fmt.Sprintf("Unable to read pools from GitHub: %s", poolErr),
			)
			return
		}

		poolDef, exists := poolsConfig.GetPool(poolID)
		if !exists {
			resp.Diagnostics.AddError(
				"Pool Not Found",
				fmt.Sprintf("Pool %q not found in pools.yaml", poolID),
			)
			return
		}

		// Get existing allocations
		allocsDB, _, allocErr := d.client.GetAllocations(ctx)
		if allocErr != nil {
			resp.Diagnostics.AddError(
				"Failed to Read Allocations",
				fmt.Sprintf("Unable to read allocations from GitHub: %s", allocErr),
			)
			return
		}

		existing := allocsDB.GetAllocationsForPool(poolID)
		cidr, err = allocator.FindNextAvailableInPool(poolDef, existing, prefixLen)

		data.ID = types.StringValue(fmt.Sprintf("next:%s:/%d", poolID, prefixLen))

	} else {
		// Mode 2: Parent CIDR sub-allocation
		parentCIDR := data.ParentCIDR.ValueString()

		// Get existing allocations
		allocsDB, _, allocErr := d.client.GetAllocations(ctx)
		if allocErr != nil {
			resp.Diagnostics.AddError(
				"Failed to Read Allocations",
				fmt.Sprintf("Unable to read allocations from GitHub: %s", allocErr),
			)
			return
		}

		// Verify parent CIDR exists as an allocation
		_, _, found := allocsDB.FindAllocationByCIDR(parentCIDR)
		if !found {
			resp.Diagnostics.AddError(
				"Parent CIDR Not Found",
				fmt.Sprintf("Parent CIDR %q not found in existing allocations", parentCIDR),
			)
			return
		}

		children := allocsDB.GetAllocationsForParent(parentCIDR)
		cidr, err = allocator.FindNextAvailableInParent(parentCIDR, children, prefixLen)

		data.ID = types.StringValue(fmt.Sprintf("next:%s:/%d", parentCIDR, prefixLen))
	}

	if err != nil {
		resp.Diagnostics.AddError(
			"No Available CIDR",
			fmt.Sprintf("Unable to find available /%d block: %s", prefixLen, err),
		)
		return
	}

	data.CIDR = types.StringValue(cidr)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

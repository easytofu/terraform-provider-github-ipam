// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package datasources

import (
	"context"
	"fmt"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/easytofu/terraform-provider-ipam-github/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var _ datasource.DataSource = &AllocationsDataSource{}
var _ datasource.DataSourceWithConfigure = &AllocationsDataSource{}

// AllocationsDataSource defines the data source implementation.
type AllocationsDataSource struct {
	client *client.GitHubClient
}

// AllocationsDataSourceModel describes the data source data model.
type AllocationsDataSourceModel struct {
	ID          types.String             `tfsdk:"id"`
	PoolID      types.String             `tfsdk:"pool_id"`
	ParentCIDR  types.String             `tfsdk:"parent_cidr"`
	Allocations []AllocationSummaryModel `tfsdk:"allocations"`
}

// AllocationSummaryModel describes an allocation summary.
type AllocationSummaryModel struct {
	ID         types.String `tfsdk:"id"`
	CIDR       types.String `tfsdk:"cidr"`
	Name       types.String `tfsdk:"name"`
	PoolID     types.String `tfsdk:"pool_id"`
	ParentCIDR types.String `tfsdk:"parent_cidr"`
	CreatedAt  types.String `tfsdk:"created_at"`
}

// NewAllocationsDataSource creates a new data source.
func NewAllocationsDataSource() datasource.DataSource {
	return &AllocationsDataSource{}
}

func (d *AllocationsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_allocations"
}

func (d *AllocationsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Lists allocations, optionally filtered by pool_id or parent_cidr.",
		MarkdownDescription: "Lists allocations from `allocations.yaml`, optionally filtered by `pool_id` or `parent_cidr`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"pool_id": schema.StringAttribute{
				Description: "Filter allocations by pool ID. Mutually exclusive with parent_cidr.",
				Optional:    true,
			},
			"parent_cidr": schema.StringAttribute{
				Description: "Filter allocations by parent CIDR. Mutually exclusive with pool_id.",
				Optional:    true,
			},
			"allocations": schema.ListNestedAttribute{
				Description: "List of allocations matching the filter criteria.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "Unique identifier for the allocation.",
							Computed:    true,
						},
						"cidr": schema.StringAttribute{
							Description: "The allocated CIDR block.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "Human-readable name for the allocation.",
							Computed:    true,
						},
						"pool_id": schema.StringAttribute{
							Description: "The pool ID this allocation belongs to.",
							Computed:    true,
						},
						"parent_cidr": schema.StringAttribute{
							Description: "The parent CIDR this allocation is carved from.",
							Computed:    true,
						},
						"created_at": schema.StringAttribute{
							Description: "Timestamp when the allocation was created.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *AllocationsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *AllocationsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data AllocationsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate mutual exclusivity
	hasPoolID := !data.PoolID.IsNull() && !data.PoolID.IsUnknown()
	hasParentCIDR := !data.ParentCIDR.IsNull() && !data.ParentCIDR.IsUnknown()

	if hasPoolID && hasParentCIDR {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"Only one of pool_id or parent_cidr can be specified, not both.",
		)
		return
	}

	// Fetch allocations from GitHub
	allocsDB, _, err := d.client.GetAllocations(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Allocations",
			fmt.Sprintf("Unable to read allocations from GitHub: %s", err),
		)
		return
	}

	// Filter allocations based on criteria
	var filtered []ipam.Allocation
	var filterID string

	if hasPoolID {
		poolID := data.PoolID.ValueString()
		filtered = allocsDB.GetAllocationsForPool(poolID)
		filterID = "pool:" + poolID
	} else if hasParentCIDR {
		parentCIDR := data.ParentCIDR.ValueString()
		filtered = allocsDB.GetAllocationsForParent(parentCIDR)
		filterID = "parent:" + parentCIDR
	} else {
		// Return all allocations
		for _, allocs := range allocsDB.Allocations {
			filtered = append(filtered, allocs...)
		}
		filterID = "all"
	}

	// Convert to data source model
	allocations := make([]AllocationSummaryModel, len(filtered))
	for i, alloc := range filtered {
		model := AllocationSummaryModel{
			ID:        types.StringValue(alloc.ID),
			CIDR:      types.StringValue(alloc.CIDR),
			Name:      types.StringValue(alloc.Name),
			CreatedAt: types.StringValue(alloc.CreatedAt),
		}

		// Set pool_id based on how we found the allocation
		if hasPoolID {
			model.PoolID = data.PoolID
		} else {
			// Try to find which pool this allocation belongs to
			for poolID, poolAllocs := range allocsDB.Allocations {
				for _, pa := range poolAllocs {
					if pa.ID == alloc.ID {
						model.PoolID = types.StringValue(poolID)
						break
					}
				}
			}
		}

		if alloc.ParentCIDR != nil {
			model.ParentCIDR = types.StringValue(*alloc.ParentCIDR)
		} else {
			model.ParentCIDR = types.StringNull()
		}

		allocations[i] = model
	}

	data.ID = types.StringValue(filterID)
	data.Allocations = allocations

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

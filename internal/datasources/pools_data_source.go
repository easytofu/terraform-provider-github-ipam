// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package datasources

import (
	"context"
	"fmt"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var _ datasource.DataSource = &PoolsDataSource{}
var _ datasource.DataSourceWithConfigure = &PoolsDataSource{}

// PoolsDataSource defines the data source implementation.
type PoolsDataSource struct {
	client *client.GitHubClient
}

// PoolsDataSourceModel describes the data source data model.
type PoolsDataSourceModel struct {
	ID    types.String       `tfsdk:"id"`
	Pools []PoolSummaryModel `tfsdk:"pools"`
}

// PoolSummaryModel describes a pool summary.
type PoolSummaryModel struct {
	PoolID      types.String `tfsdk:"pool_id"`
	Description types.String `tfsdk:"description"`
	CIDRs       types.List   `tfsdk:"cidrs"`
}

// NewPoolsDataSource creates a new data source.
func NewPoolsDataSource() datasource.DataSource {
	return &PoolsDataSource{}
}

func (d *PoolsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pools"
}

func (d *PoolsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Lists all IP pools defined in pools.yaml.",
		MarkdownDescription: "Lists all IP pools defined in `pools.yaml`. This data source provides a summary of available pools for allocation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"pools": schema.ListNestedAttribute{
				Description:         "List of IP pools.",
				MarkdownDescription: "List of IP pools defined in `pools.yaml`.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"pool_id": schema.StringAttribute{
							Description: "Unique identifier for the pool.",
							Computed:    true,
						},
						"description": schema.StringAttribute{
							Description: "Human-readable description of the pool.",
							Computed:    true,
						},
						"cidrs": schema.ListAttribute{
							Description: "CIDR ranges available in this pool.",
							ElementType: types.StringType,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *PoolsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PoolsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PoolsDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fetch pools from GitHub
	poolsConfig, err := d.client.GetPools(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Pools",
			fmt.Sprintf("Unable to read pools from GitHub: %s", err),
		)
		return
	}

	// Convert to data source model
	pools := make([]PoolSummaryModel, 0, len(poolsConfig.Pools))
	for poolID, poolDef := range poolsConfig.Pools {
		// Convert CIDRs to types.List
		cidrValues := make([]types.String, len(poolDef.CIDR))
		for i, cidr := range poolDef.CIDR {
			cidrValues[i] = types.StringValue(cidr)
		}
		cidrs, diags := types.ListValueFrom(ctx, types.StringType, poolDef.CIDR)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		pools = append(pools, PoolSummaryModel{
			PoolID:      types.StringValue(poolID),
			Description: types.StringValue(poolDef.Description),
			CIDRs:       cidrs,
		})
	}

	data.ID = types.StringValue("pools")
	data.Pools = pools

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

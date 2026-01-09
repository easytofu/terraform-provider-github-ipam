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
var _ datasource.DataSource = &PoolDataSource{}
var _ datasource.DataSourceWithConfigure = &PoolDataSource{}

// PoolDataSource defines the data source implementation.
type PoolDataSource struct {
	client *client.GitHubClient
}

// PoolDataSourceModel describes the data source data model.
type PoolDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	PoolID      types.String `tfsdk:"pool_id"`
	Description types.String `tfsdk:"description"`
	CIDRs       types.List   `tfsdk:"cidrs"`
	Metadata    types.Map    `tfsdk:"metadata"`
}

// NewPoolDataSource creates a new data source.
func NewPoolDataSource() datasource.DataSource {
	return &PoolDataSource{}
}

func (d *PoolDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (d *PoolDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Retrieves details about a specific IP pool.",
		MarkdownDescription: "Retrieves details about a specific IP pool defined in `pools.yaml`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"pool_id": schema.StringAttribute{
				Description: "The pool identifier to look up.",
				Required:    true,
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
			"metadata": schema.MapAttribute{
				Description: "Arbitrary metadata associated with the pool.",
				ElementType: types.StringType,
				Computed:    true,
			},
		},
	}
}

func (d *PoolDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PoolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PoolDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolID := data.PoolID.ValueString()

	// Fetch pools from GitHub
	poolsConfig, err := d.client.GetPools(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Pools",
			fmt.Sprintf("Unable to read pools from GitHub: %s", err),
		)
		return
	}

	// Find the specific pool
	poolDef, exists := poolsConfig.GetPool(poolID)
	if !exists {
		resp.Diagnostics.AddError(
			"Pool Not Found",
			fmt.Sprintf("Pool with ID %q not found in pools.yaml", poolID),
		)
		return
	}

	// Convert CIDRs to types.List
	cidrs, diags := types.ListValueFrom(ctx, types.StringType, poolDef.CIDR)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Convert metadata to types.Map
	var metadata types.Map
	if len(poolDef.Metadata) > 0 {
		metadata, diags = types.MapValueFrom(ctx, types.StringType, poolDef.Metadata)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		metadata = types.MapNull(types.StringType)
	}

	data.ID = types.StringValue(poolID)
	data.Description = types.StringValue(poolDef.Description)
	data.CIDRs = cidrs
	data.Metadata = metadata

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

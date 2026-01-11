// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package datasources

import (
	"context"
	"fmt"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var _ datasource.DataSource = &AllocationDataSource{}
var _ datasource.DataSourceWithConfigure = &AllocationDataSource{}

// AllocationDataSource defines the data source implementation.
type AllocationDataSource struct {
	client *client.GitHubClient
}

// AllocationDataSourceModel describes the data source data model.
type AllocationDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	CIDR       types.String `tfsdk:"cidr"`
	PoolID     types.String `tfsdk:"pool_id"`
	ParentCIDR types.String `tfsdk:"parent_cidr"`
	Metadata   types.Map    `tfsdk:"metadata"`
}

// NewAllocationDataSource creates a new data source.
func NewAllocationDataSource() datasource.DataSource {
	return &AllocationDataSource{}
}

func (d *AllocationDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_allocation"
}

func (d *AllocationDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a specific allocation by ID or name.",
		MarkdownDescription: `Looks up a specific allocation by ID or name.

This data source is useful when you need to reference an allocation created by another
Terraform workspace or process.

**Example:**
` + "```hcl" + `
data "github-ipam_allocation" "vpc" {
  name = "vpc-stake-stake-can"
}

resource "aws_vpc" "main" {
  cidr_block = data.github-ipam_allocation.vpc.cidr
}
` + "```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:         "Allocation ID (UUID). Either id or name must be specified.",
				MarkdownDescription: "Allocation ID (UUID). Either `id` or `name` must be specified.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.Expressions{
						path.MatchRoot("id"),
						path.MatchRoot("name"),
					}...),
				},
			},
			"name": schema.StringAttribute{
				Description:         "Allocation name. Either id or name must be specified.",
				MarkdownDescription: "Allocation name. Either `id` or `name` must be specified.",
				Optional:            true,
				Computed:            true,
			},
			"cidr": schema.StringAttribute{
				Description:         "The allocated CIDR block.",
				MarkdownDescription: "The allocated CIDR block.",
				Computed:            true,
			},
			"pool_id": schema.StringAttribute{
				Description:         "The pool this allocation belongs to.",
				MarkdownDescription: "The pool this allocation belongs to.",
				Computed:            true,
			},
			"parent_cidr": schema.StringAttribute{
				Description:         "Parent CIDR if this is a sub-allocation.",
				MarkdownDescription: "Parent CIDR if this is a sub-allocation.",
				Computed:            true,
			},
			"metadata": schema.MapAttribute{
				Description:         "Key-value metadata for the allocation.",
				MarkdownDescription: "Key-value metadata for the allocation.",
				ElementType:         types.StringType,
				Computed:            true,
			},
		},
	}
}

func (d *AllocationDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	ghClient, ok := req.ProviderData.(*client.GitHubClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.GitHubClient, got: %T.", req.ProviderData),
		)
		return
	}

	d.client = ghClient
}

func (d *AllocationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config AllocationDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, _, err := d.client.GetAllocations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read allocations", err.Error())
		return
	}

	var found bool

	if !config.ID.IsNull() && config.ID.ValueString() != "" {
		// Look up by ID
		alloc, poolID, ok := db.FindAllocationByID(config.ID.ValueString())
		if ok {
			found = true
			config.Name = types.StringValue(alloc.Name)
			config.CIDR = types.StringValue(alloc.CIDR)
			config.PoolID = types.StringValue(poolID)
			if alloc.ParentCIDR != nil {
				config.ParentCIDR = types.StringValue(*alloc.ParentCIDR)
			} else {
				config.ParentCIDR = types.StringNull()
			}
			if len(alloc.Metadata) > 0 {
				metadataValue, diags := types.MapValueFrom(ctx, types.StringType, alloc.Metadata)
				resp.Diagnostics.Append(diags...)
				config.Metadata = metadataValue
			} else {
				config.Metadata = types.MapNull(types.StringType)
			}
		}
	} else if !config.Name.IsNull() && config.Name.ValueString() != "" {
		// Look up by name
		alloc, poolID, ok := db.FindAllocationByName(config.Name.ValueString())
		if ok {
			found = true
			config.ID = types.StringValue(alloc.ID)
			config.CIDR = types.StringValue(alloc.CIDR)
			config.PoolID = types.StringValue(poolID)
			if alloc.ParentCIDR != nil {
				config.ParentCIDR = types.StringValue(*alloc.ParentCIDR)
			} else {
				config.ParentCIDR = types.StringNull()
			}
			if len(alloc.Metadata) > 0 {
				metadataValue, diags := types.MapValueFrom(ctx, types.StringType, alloc.Metadata)
				resp.Diagnostics.Append(diags...)
				config.Metadata = metadataValue
			} else {
				config.Metadata = types.MapNull(types.StringType)
			}
		}
	}

	if !found {
		if !config.ID.IsNull() {
			resp.Diagnostics.AddError("Allocation not found", fmt.Sprintf("No allocation found with ID %q", config.ID.ValueString()))
		} else {
			resp.Diagnostics.AddError("Allocation not found", fmt.Sprintf("No allocation found with name %q", config.Name.ValueString()))
		}
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

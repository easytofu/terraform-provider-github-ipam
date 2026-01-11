// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/easytofu/terraform-provider-ipam-github/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &PoolResource{}
	_ resource.ResourceWithConfigure   = &PoolResource{}
	_ resource.ResourceWithImportState = &PoolResource{}
)

// NewPoolResource creates a new pool resource.
func NewPoolResource() resource.Resource {
	return &PoolResource{}
}

// PoolResource defines the resource implementation.
type PoolResource struct {
	client *client.GitHubClient
}

// PoolResourceModel describes the resource data model.
type PoolResourceModel struct {
	ID          types.String `tfsdk:"id"`
	CIDR        types.List   `tfsdk:"cidr"`
	Description types.String `tfsdk:"description"`
	Metadata    types.Map    `tfsdk:"metadata"`
}

func (r *PoolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (r *PoolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IPAM pool definition. Pools define the CIDR ranges " +
			"from which allocations can be made.",
		MarkdownDescription: `Manages an IPAM pool definition in pools.yaml.

Pools define the CIDR ranges from which allocations can be made. Each pool can have
multiple CIDR ranges, a description, and arbitrary metadata.

**Example:**
` + "```hcl" + `
resource "github-ipam_pool" "production" {
  id          = "production"
  cidr        = ["10.0.0.0/8", "172.16.0.0/12"]
  description = "Production network pool"
  metadata = {
    environment = "production"
    team        = "platform"
  }
}
` + "```",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:            true,
				Description:         "Unique identifier for this pool (used as pool_id in allocations).",
				MarkdownDescription: "Unique identifier for this pool (used as `pool_id` in allocations).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cidr": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				Description:         "List of CIDR ranges for this pool.",
				MarkdownDescription: "List of CIDR ranges for this pool (e.g., `[\"10.0.0.0/8\", \"172.16.0.0/12\"]`).",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Description:         "Human-readable description of the pool.",
				MarkdownDescription: "Human-readable description of the pool.",
			},
			"metadata": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				Description:         "Key-value metadata for the pool.",
				MarkdownDescription: "Key-value metadata for the pool.",
			},
		},
	}
}

func (r *PoolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	ghClient, ok := req.ProviderData.(*client.GitHubClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.GitHubClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = ghClient
}

func (r *PoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolID := plan.ID.ValueString()

	tflog.Debug(ctx, "Creating pool", map[string]interface{}{
		"pool_id": poolID,
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		// Read pools.yaml with SHA for OCC
		pools, sha, err := r.client.GetPoolsWithSHA(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read pools: %w", err)
		}

		// Check if pool already exists
		if _, exists := pools.GetPool(poolID); exists {
			return false, fmt.Errorf("pool %s already exists", poolID)
		}

		// Build CIDR list
		var cidrList []string
		resp.Diagnostics.Append(plan.CIDR.ElementsAs(ctx, &cidrList, false)...)
		if resp.Diagnostics.HasError() {
			return false, fmt.Errorf("failed to parse CIDR list")
		}

		// Build metadata map
		metadata := make(map[string]string)
		if !plan.Metadata.IsNull() {
			resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
			if resp.Diagnostics.HasError() {
				return false, fmt.Errorf("failed to parse metadata")
			}
		}

		poolDef := ipam.PoolDefinition{
			CIDR:        cidrList,
			Description: plan.Description.ValueString(),
			Metadata:    metadata,
		}

		// Validate the new pool
		testPools := ipam.NewPoolsConfig()
		testPools.Pools = make(map[string]ipam.PoolDefinition)
		for k, v := range pools.Pools {
			testPools.Pools[k] = v
		}
		testPools.AddPool(poolID, poolDef)
		if err := testPools.ValidatePools(); err != nil {
			return false, fmt.Errorf("pool validation failed: %w", err)
		}

		pools.AddPool(poolID, poolDef)

		commitMsg := fmt.Sprintf("ipam: create pool %s", poolID)
		err = r.client.UpdatePools(ctx, pools, sha, commitMsg)
		if r.client.IsConflictError(err) {
			tflog.Debug(ctx, "Conflict detected, will retry", map[string]interface{}{
				"attempt": attempt,
			})
			return true, err
		}
		return false, err
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to create pool", err.Error())
		return
	}

	tflog.Info(ctx, "Created pool", map[string]interface{}{
		"pool_id": poolID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *PoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolID := state.ID.ValueString()

	tflog.Debug(ctx, "Reading pool", map[string]interface{}{
		"pool_id": poolID,
	})

	pools, err := r.client.GetPools(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read pools", err.Error())
		return
	}

	poolDef, exists := pools.GetPool(poolID)
	if !exists {
		tflog.Warn(ctx, "Pool not found, removing from state", map[string]interface{}{
			"pool_id": poolID,
		})
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with current values
	cidrValue, diags := types.ListValueFrom(ctx, types.StringType, poolDef.CIDR)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.CIDR = cidrValue

	state.Description = types.StringValue(poolDef.Description)

	if len(poolDef.Metadata) > 0 {
		metadataValue, diags := types.MapValueFrom(ctx, types.StringType, poolDef.Metadata)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Metadata = metadataValue
	} else {
		state.Metadata = types.MapNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *PoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolID := plan.ID.ValueString()

	tflog.Debug(ctx, "Updating pool", map[string]interface{}{
		"pool_id": poolID,
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		pools, sha, err := r.client.GetPoolsWithSHA(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read pools: %w", err)
		}

		if _, exists := pools.GetPool(poolID); !exists {
			return false, fmt.Errorf("pool %s not found", poolID)
		}

		// Build CIDR list
		var cidrList []string
		resp.Diagnostics.Append(plan.CIDR.ElementsAs(ctx, &cidrList, false)...)
		if resp.Diagnostics.HasError() {
			return false, fmt.Errorf("failed to parse CIDR list")
		}

		// Build metadata map
		metadata := make(map[string]string)
		if !plan.Metadata.IsNull() {
			resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
			if resp.Diagnostics.HasError() {
				return false, fmt.Errorf("failed to parse metadata")
			}
		}

		poolDef := ipam.PoolDefinition{
			CIDR:        cidrList,
			Description: plan.Description.ValueString(),
			Metadata:    metadata,
		}

		// Validate the updated pool
		testPools := ipam.NewPoolsConfig()
		testPools.Pools = make(map[string]ipam.PoolDefinition)
		for k, v := range pools.Pools {
			if k != poolID {
				testPools.Pools[k] = v
			}
		}
		testPools.AddPool(poolID, poolDef)
		if err := testPools.ValidatePools(); err != nil {
			return false, fmt.Errorf("pool validation failed: %w", err)
		}

		pools.AddPool(poolID, poolDef)

		commitMsg := fmt.Sprintf("ipam: update pool %s", poolID)
		err = r.client.UpdatePools(ctx, pools, sha, commitMsg)
		if r.client.IsConflictError(err) {
			return true, err
		}
		return false, err
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to update pool", err.Error())
		return
	}

	tflog.Info(ctx, "Updated pool", map[string]interface{}{
		"pool_id": poolID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *PoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolID := state.ID.ValueString()

	tflog.Debug(ctx, "Deleting pool", map[string]interface{}{
		"pool_id": poolID,
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		pools, sha, err := r.client.GetPoolsWithSHA(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read pools: %w", err)
		}

		if _, exists := pools.GetPool(poolID); !exists {
			// Already deleted
			tflog.Debug(ctx, "Pool already deleted", map[string]interface{}{
				"pool_id": poolID,
			})
			return false, nil
		}

		// Check for existing allocations in this pool
		db, _, err := r.client.GetAllocations(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read allocations: %w", err)
		}

		allocs := db.GetAllocationsForPool(poolID)
		if len(allocs) > 0 {
			return false, fmt.Errorf("cannot delete pool %s: has %d active allocations", poolID, len(allocs))
		}

		if err := pools.RemovePool(poolID); err != nil {
			return false, err
		}

		commitMsg := fmt.Sprintf("ipam: delete pool %s", poolID)
		err = r.client.UpdatePools(ctx, pools, sha, commitMsg)
		if r.client.IsConflictError(err) {
			return true, err
		}
		return false, err
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to delete pool", err.Error())
		return
	}

	tflog.Info(ctx, "Deleted pool", map[string]interface{}{
		"pool_id": poolID,
	})
}

func (r *PoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Debug(ctx, "Importing pool", map[string]interface{}{
		"id": req.ID,
	})

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

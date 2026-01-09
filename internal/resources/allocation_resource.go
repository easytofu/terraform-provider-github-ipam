// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/easytofu/terraform-provider-ipam-github/internal/ipam"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &AllocationResource{}
	_ resource.ResourceWithConfigure   = &AllocationResource{}
	_ resource.ResourceWithImportState = &AllocationResource{}
)

// NewAllocationResource creates a new allocation resource.
func NewAllocationResource() resource.Resource {
	return &AllocationResource{}
}

// AllocationResource defines the resource implementation.
type AllocationResource struct {
	client    *client.GitHubClient
	allocator *ipam.Allocator
}

// AllocationResourceModel describes the resource data model.
type AllocationResourceModel struct {
	ID         types.String `tfsdk:"id"`
	PoolID     types.String `tfsdk:"pool_id"`
	ParentCIDR types.String `tfsdk:"parent_cidr"`
	CIDRMask   types.Int64  `tfsdk:"cidr_mask"`
	CIDR       types.String `tfsdk:"cidr"`
	Name       types.String `tfsdk:"name"`
	Metadata   types.Map    `tfsdk:"metadata"`
}

func (r *AllocationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_allocation"
}

func (r *AllocationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Allocates a CIDR block from an IPAM pool (pool_id) or from an existing " +
			"allocation (parent_cidr). Exactly one of pool_id or parent_cidr must be specified.",
		MarkdownDescription: `Allocates a CIDR block from an IPAM pool or from an existing allocation.

**Two allocation modes:**
- **Mode 1 (pool_id)**: Allocate from a pool defined in pools.yaml
- **Mode 2 (parent_cidr)**: Sub-allocate from an existing allocation (e.g., subnets within a VPC)

Exactly one of ` + "`pool_id`" + ` or ` + "`parent_cidr`" + ` must be specified.`,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Unique identifier for this allocation (UUID).",
				MarkdownDescription: "Unique identifier for this allocation (UUID).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pool_id": schema.StringAttribute{
				Optional:            true,
				Description:         "Pool ID from pools.yaml to allocate from (Mode 1). Mutually exclusive with parent_cidr.",
				MarkdownDescription: "Pool ID from pools.yaml to allocate from (Mode 1). Mutually exclusive with `parent_cidr`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.Expressions{
						path.MatchRoot("pool_id"),
						path.MatchRoot("parent_cidr"),
					}...),
				},
			},
			"parent_cidr": schema.StringAttribute{
				Optional: true,
				Description: "CIDR of an existing allocation to sub-allocate from (Mode 2). " +
					"Use this for allocating subnets within a VPC CIDR. Mutually exclusive with pool_id.",
				MarkdownDescription: "CIDR of an existing allocation to sub-allocate from (Mode 2). " +
					"Use this for allocating subnets within a VPC CIDR. Mutually exclusive with `pool_id`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cidr_mask": schema.Int64Attribute{
				Required:            true,
				Description:         "Prefix length for the allocation (e.g., 16 for /16, 24 for /24).",
				MarkdownDescription: "Prefix length for the allocation (e.g., `16` for /16, `24` for /24).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"cidr": schema.StringAttribute{
				Computed:            true,
				Description:         "Allocated CIDR block (computed at apply time).",
				MarkdownDescription: "Allocated CIDR block (computed at apply time).",
				// CRITICAL: This remains Unknown during plan phase
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "Human-readable name for this allocation.",
				MarkdownDescription: "Human-readable name for this allocation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"metadata": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				Description:         "Key-value metadata for the allocation.",
				MarkdownDescription: "Key-value metadata for the allocation.",
			},
		},
	}
}

func (r *AllocationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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
	r.allocator = ipam.NewAllocator()
}

func (r *AllocationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AllocationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate stable UUID for this allocation
	allocationID := uuid.New().String()

	tflog.Debug(ctx, "Creating allocation", map[string]interface{}{
		"allocation_id": allocationID,
		"name":          plan.Name.ValueString(),
		"cidr_mask":     plan.CIDRMask.ValueInt64(),
	})

	var allocatedCIDR string
	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		// Read pools.yaml (read-only)
		pools, err := r.client.GetPools(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read pools: %w", err)
		}

		// Read allocations.json with SHA for OCC
		db, sha, err := r.client.GetAllocations(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read allocations: %w", err)
		}

		// Idempotency check: see if this ID already exists
		if existing, _, found := db.FindAllocationByID(allocationID); found {
			allocatedCIDR = existing.CIDR
			tflog.Debug(ctx, "Allocation already exists (idempotent)", map[string]interface{}{
				"allocation_id": allocationID,
				"cidr":          allocatedCIDR,
			})
			return false, nil
		}

		var newCIDR string
		var poolID string

		if !plan.PoolID.IsNull() {
			// Mode 1: Allocate from pool defined in pools.yaml
			poolID = plan.PoolID.ValueString()
			poolDef, exists := pools.GetPool(poolID)
			if !exists {
				return false, fmt.Errorf("pool_id %q not found in pools.yaml", poolID)
			}

			existingAllocs := db.GetAllocationsForPool(poolID)
			newCIDR, err = r.allocator.FindNextAvailableInPool(poolDef, existingAllocs, int(plan.CIDRMask.ValueInt64()))
			if err != nil {
				return false, fmt.Errorf("allocation from pool %s failed: %w", poolID, err)
			}

			tflog.Debug(ctx, "Allocated from pool", map[string]interface{}{
				"pool_id": poolID,
				"cidr":    newCIDR,
			})
		} else {
			// Mode 2: Sub-allocate from parent_cidr
			parentCIDR := plan.ParentCIDR.ValueString()

			// Find which pool the parent belongs to
			parentAlloc, parentPoolID, found := db.FindAllocationByCIDR(parentCIDR)
			if !found {
				return false, fmt.Errorf("parent_cidr %q not found in allocations", parentCIDR)
			}
			poolID = parentPoolID
			_ = parentAlloc // Can use for validation if needed

			childAllocs := db.GetAllocationsForParent(parentCIDR)
			newCIDR, err = r.allocator.FindNextAvailableInParent(parentCIDR, childAllocs, int(plan.CIDRMask.ValueInt64()))
			if err != nil {
				return false, fmt.Errorf("sub-allocation from %s failed: %w", parentCIDR, err)
			}

			tflog.Debug(ctx, "Sub-allocated from parent", map[string]interface{}{
				"parent_cidr": parentCIDR,
				"cidr":        newCIDR,
			})
		}

		// Build metadata map
		metadata := make(map[string]string)
		if !plan.Metadata.IsNull() {
			resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
			if resp.Diagnostics.HasError() {
				return false, fmt.Errorf("failed to parse metadata")
			}
		}

		// Build parent CIDR pointer
		var parentCIDRPtr *string
		if !plan.ParentCIDR.IsNull() {
			pc := plan.ParentCIDR.ValueString()
			parentCIDRPtr = &pc
		}

		allocation := ipam.Allocation{
			CIDR:       newCIDR,
			ID:         allocationID,
			Name:       plan.Name.ValueString(),
			ParentCIDR: parentCIDRPtr,
			Metadata:   metadata,
		}

		db.AddAllocation(poolID, allocation)

		commitMsg := fmt.Sprintf("ipam: allocate %s (%s)", newCIDR, plan.Name.ValueString())
		err = r.client.UpdateAllocations(ctx, db, sha, commitMsg)
		if r.client.IsConflictError(err) {
			tflog.Debug(ctx, "Conflict detected, will retry", map[string]interface{}{
				"attempt": attempt,
			})
			return true, err // Retry on conflict
		}

		if err == nil {
			allocatedCIDR = newCIDR
		}
		return false, err
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to allocate CIDR", err.Error())
		return
	}

	plan.ID = types.StringValue(allocationID)
	plan.CIDR = types.StringValue(allocatedCIDR)

	tflog.Info(ctx, "Created allocation", map[string]interface{}{
		"id":   allocationID,
		"cidr": allocatedCIDR,
		"name": plan.Name.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AllocationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AllocationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading allocation", map[string]interface{}{
		"id": state.ID.ValueString(),
	})

	db, _, err := r.client.GetAllocations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read allocations", err.Error())
		return
	}

	alloc, poolID, found := db.FindAllocationByID(state.ID.ValueString())
	if !found {
		tflog.Warn(ctx, "Allocation not found, removing from state", map[string]interface{}{
			"id": state.ID.ValueString(),
		})
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with current values from the database
	state.CIDR = types.StringValue(alloc.CIDR)
	state.Name = types.StringValue(alloc.Name)

	if alloc.ParentCIDR != nil {
		state.ParentCIDR = types.StringValue(*alloc.ParentCIDR)
	} else {
		// If no parent CIDR, set pool_id
		state.PoolID = types.StringValue(poolID)
	}

	// Update metadata if present
	if len(alloc.Metadata) > 0 {
		metadataValue, diags := types.MapValueFrom(ctx, types.StringType, alloc.Metadata)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Metadata = metadataValue
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *AllocationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AllocationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only metadata can be updated (pool_id, parent_cidr, cidr_mask, name all require replace)
	tflog.Debug(ctx, "Updating allocation metadata", map[string]interface{}{
		"id": plan.ID.ValueString(),
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		db, sha, err := r.client.GetAllocations(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read allocations: %w", err)
		}

		alloc, poolID, found := db.FindAllocationByID(plan.ID.ValueString())
		if !found {
			return false, fmt.Errorf("allocation %s not found", plan.ID.ValueString())
		}

		// Update metadata
		metadata := make(map[string]string)
		if !plan.Metadata.IsNull() {
			resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
			if resp.Diagnostics.HasError() {
				return false, fmt.Errorf("failed to parse metadata")
			}
		}
		alloc.Metadata = metadata

		// Remove old and add updated allocation
		if err := db.RemoveAllocation(poolID, alloc.ID); err != nil {
			return false, err
		}
		db.AddAllocation(poolID, *alloc)

		commitMsg := fmt.Sprintf("ipam: update %s (%s)", alloc.CIDR, alloc.Name)
		err = r.client.UpdateAllocations(ctx, db, sha, commitMsg)
		if r.client.IsConflictError(err) {
			return true, err
		}
		return false, err
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to update allocation", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *AllocationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AllocationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting allocation", map[string]interface{}{
		"id":   state.ID.ValueString(),
		"cidr": state.CIDR.ValueString(),
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		db, sha, err := r.client.GetAllocations(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read allocations: %w", err)
		}

		// Find and remove the allocation
		_, poolID, found := db.FindAllocationByID(state.ID.ValueString())
		if !found {
			// Already deleted
			tflog.Debug(ctx, "Allocation already deleted", map[string]interface{}{
				"id": state.ID.ValueString(),
			})
			return false, nil
		}

		// Check for child allocations
		childAllocs := db.GetAllocationsForParent(state.CIDR.ValueString())
		if len(childAllocs) > 0 {
			return false, fmt.Errorf("cannot delete allocation %s: has %d child allocations", state.CIDR.ValueString(), len(childAllocs))
		}

		if err := db.RemoveAllocation(poolID, state.ID.ValueString()); err != nil {
			return false, err
		}

		commitMsg := fmt.Sprintf("ipam: deallocate %s (%s)", state.CIDR.ValueString(), state.Name.ValueString())
		err = r.client.UpdateAllocations(ctx, db, sha, commitMsg)
		if r.client.IsConflictError(err) {
			return true, err
		}
		return false, err
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to deallocate CIDR", err.Error())
		return
	}

	tflog.Info(ctx, "Deleted allocation", map[string]interface{}{
		"id":   state.ID.ValueString(),
		"cidr": state.CIDR.ValueString(),
	})
}

func (r *AllocationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by allocation ID
	tflog.Debug(ctx, "Importing allocation", map[string]interface{}{
		"id": req.ID,
	})

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

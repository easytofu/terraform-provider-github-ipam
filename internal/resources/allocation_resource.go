// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"net"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/easytofu/terraform-provider-ipam-github/internal/ipam"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	ID             types.String `tfsdk:"id"`
	PoolID         types.String `tfsdk:"pool_id"`
	ParentCIDR     types.String `tfsdk:"parent_cidr"`
	CIDRMask       types.Int64  `tfsdk:"cidr_mask"`
	CIDR           types.String `tfsdk:"cidr"`
	Name           types.String `tfsdk:"name"`
	Status         types.String `tfsdk:"status"`
	ContiguousWith types.String `tfsdk:"contiguous_with"`
	Metadata       types.Map    `tfsdk:"metadata"`
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "Human-readable name for this allocation. Can be updated in-place.",
				MarkdownDescription: "Human-readable name for this allocation. Can be updated in-place.",
			},
			"status": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "Status of the allocation: 'allocation' (default) or 'reservation'. Reservations cannot be used for sub-allocations.",
				MarkdownDescription: "Status of the allocation: `allocation` (default) or `reservation`. Reservations hold space for future use.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("allocation", "reservation"),
				},
			},
			"contiguous_with": schema.StringAttribute{
				Optional: true,
				Description: "CIDR of an existing allocation that this block must be immediately adjacent to. " +
					"If the constraint cannot be satisfied, the plan will fail.",
				MarkdownDescription: "CIDR of an existing allocation that this block must be immediately adjacent to. " +
					"If the constraint cannot be satisfied, the plan will fail.",
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

	// Generate deterministic UUID for this allocation based on stable configuration
	// This ensures idempotency across Terraform retries
	var idInput string
	if !plan.PoolID.IsNull() {
		idInput = fmt.Sprintf("pool:%s:name:%s:mask:%d", plan.PoolID.ValueString(), plan.Name.ValueString(), plan.CIDRMask.ValueInt64())
	} else {
		idInput = fmt.Sprintf("parent:%s:name:%s:mask:%d", plan.ParentCIDR.ValueString(), plan.Name.ValueString(), plan.CIDRMask.ValueInt64())
	}
	// Use UUID v5 with DNS namespace for deterministic generation
	allocationID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(idInput)).String()

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

		// Check for duplicate name
		if existing, _, found := db.FindAllocationByName(plan.Name.ValueString()); found {
			return false, fmt.Errorf("allocation name %q already exists (used by allocation %s)", plan.Name.ValueString(), existing.CIDR)
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

			// Check if pool is reserved - cannot allocate from reserved pools
			if poolDef.Reserved {
				return false, fmt.Errorf("cannot allocate from pool %q: pool is reserved (reserved pools cannot have allocations)", poolID)
			}

			existingAllocs := db.GetAllocationsForPool(poolID)

			// Check if contiguous_with is specified
			if !plan.ContiguousWith.IsNull() {
				targetCIDR := plan.ContiguousWith.ValueString()
				newCIDR, err = findContiguousCIDR(poolDef, existingAllocs, int(plan.CIDRMask.ValueInt64()), targetCIDR)
				if err != nil {
					return false, fmt.Errorf("contiguous allocation failed: %w", err)
				}
			} else {
				newCIDR, err = r.allocator.FindNextAvailableInPool(poolDef, existingAllocs, int(plan.CIDRMask.ValueInt64()))
				if err != nil {
					return false, fmt.Errorf("allocation from pool %s failed: %w", poolID, err)
				}
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

			// Check if parent is reserved - cannot sub-allocate from reserved blocks
			if parentAlloc.Reserved {
				return false, fmt.Errorf("cannot sub-allocate from %q: parent is a reservation (reserved blocks cannot have children)", parentCIDR)
			}

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
			diags := plan.Metadata.ElementsAs(ctx, &metadata, false)
			resp.Diagnostics.Append(diags...)
			if diags.HasError() {
				return false, fmt.Errorf("failed to parse metadata: %s", diagnosticsToString(diags))
			}
		}

		// Build parent CIDR pointer
		var parentCIDRPtr *string
		if !plan.ParentCIDR.IsNull() {
			pc := plan.ParentCIDR.ValueString()
			parentCIDRPtr = &pc
		}

		// Determine status (default to "allocation")
		status := "allocation"
		isReserved := false
		if !plan.Status.IsNull() && plan.Status.ValueString() == "reservation" {
			status = "reservation"
			isReserved = true
		}

		// Build contiguous_with pointer
		var contiguousWithPtr *string
		if !plan.ContiguousWith.IsNull() {
			cw := plan.ContiguousWith.ValueString()
			contiguousWithPtr = &cw
		}

		allocation := ipam.Allocation{
			CIDR:           newCIDR,
			ID:             allocationID,
			Name:           plan.Name.ValueString(),
			ParentCIDR:     parentCIDRPtr,
			Metadata:       metadata,
			Reserved:       isReserved,
			ContiguousWith: contiguousWithPtr,
		}
		_ = status // Used for logging

		db.AddAllocation(poolID, allocation)

		action := "allocate"
		if isReserved {
			action = "reserve"
		}
		commitMsg := fmt.Sprintf("ipam: %s %s (%s)", action, newCIDR, plan.Name.ValueString())
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
	if plan.Status.IsNull() || plan.Status.IsUnknown() {
		plan.Status = types.StringValue("allocation")
	}

	tflog.Info(ctx, "Created allocation", map[string]interface{}{
		"id":     allocationID,
		"cidr":   allocatedCIDR,
		"name":   plan.Name.ValueString(),
		"status": plan.Status.ValueString(),
	})

	// Regenerate README (best effort, don't fail on error)
	if err := r.client.RegenerateREADME(ctx); err != nil {
		tflog.Warn(ctx, "Failed to regenerate README", map[string]interface{}{
			"error": err.Error(),
		})
	}

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

	// Set status based on Reserved flag
	if alloc.Reserved {
		state.Status = types.StringValue("reservation")
	} else {
		state.Status = types.StringValue("allocation")
	}

	// Set contiguous_with if present
	if alloc.ContiguousWith != nil {
		state.ContiguousWith = types.StringValue(*alloc.ContiguousWith)
	}

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

	// Name, metadata, and status can be updated in-place
	tflog.Debug(ctx, "Updating allocation", map[string]interface{}{
		"id":   plan.ID.ValueString(),
		"name": plan.Name.ValueString(),
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	// Capture the CIDR from the database to set in state after update
	var allocCIDR string

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		db, sha, err := r.client.GetAllocations(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read allocations: %w", err)
		}

		alloc, poolID, found := db.FindAllocationByID(plan.ID.ValueString())
		if !found {
			return false, fmt.Errorf("allocation %s not found", plan.ID.ValueString())
		}

		// Capture CIDR for state update
		allocCIDR = alloc.CIDR

		// Check for duplicate name before making any changes
		newName := plan.Name.ValueString()
		if newName != alloc.Name {
			if existing, _, found := db.FindAllocationByName(newName); found && existing.ID != alloc.ID {
				return false, fmt.Errorf("cannot rename allocation to %q: name already exists (used by allocation %s)", newName, existing.CIDR)
			}
		}

		// Update name
		alloc.Name = newName

		// Update metadata
		metadata := make(map[string]string)
		if !plan.Metadata.IsNull() {
			diags := plan.Metadata.ElementsAs(ctx, &metadata, false)
			resp.Diagnostics.Append(diags...)
			if diags.HasError() {
				return false, fmt.Errorf("failed to parse metadata: %s", diagnosticsToString(diags))
			}
		}
		alloc.Metadata = metadata

		// Update status (allows converting between allocation and reservation)
		if !plan.Status.IsNull() {
			alloc.Reserved = plan.Status.ValueString() == "reservation"
		}

		// Remove old and add updated allocation
		if err := db.RemoveAllocation(poolID, alloc.ID); err != nil {
			return false, err
		}
		db.AddAllocation(poolID, *alloc)

		action := "update"
		if alloc.Reserved {
			action = "update reservation"
		}
		commitMsg := fmt.Sprintf("ipam: %s %s (%s)", action, alloc.CIDR, plan.Name.ValueString())
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

	// Set the CIDR from the database (it's immutable, so always use the stored value)
	plan.CIDR = types.StringValue(allocCIDR)

	// Regenerate README (best effort, don't fail on error)
	if err := r.client.RegenerateREADME(ctx); err != nil {
		tflog.Warn(ctx, "Failed to regenerate README", map[string]interface{}{
			"error": err.Error(),
		})
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

	// Regenerate README (best effort, don't fail on error)
	if err := r.client.RegenerateREADME(ctx); err != nil {
		tflog.Warn(ctx, "Failed to regenerate README", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

func (r *AllocationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Debug(ctx, "Importing allocation", map[string]interface{}{
		"id": req.ID,
	})

	// Read the allocation to get all necessary fields
	db, _, err := r.client.GetAllocations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read allocations", err.Error())
		return
	}

	alloc, poolID, found := db.FindAllocationByID(req.ID)
	if !found {
		resp.Diagnostics.AddError("Allocation not found", fmt.Sprintf("No allocation with ID %q exists", req.ID))
		return
	}

	// Parse CIDR to extract mask
	_, network, err := net.ParseCIDR(alloc.CIDR)
	if err != nil {
		resp.Diagnostics.AddError("Invalid CIDR in allocation", fmt.Sprintf("CIDR %q is invalid: %s", alloc.CIDR, err))
		return
	}
	maskSize, _ := network.Mask.Size()

	// Set all state attributes
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cidr"), alloc.CIDR)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cidr_mask"), int64(maskSize))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), alloc.Name)...)

	// Set status
	status := "allocation"
	if alloc.Reserved {
		status = "reservation"
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status"), status)...)

	// Set pool_id or parent_cidr based on allocation type
	if alloc.ParentCIDR != nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("parent_cidr"), *alloc.ParentCIDR)...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), poolID)...)
	}

	// Set contiguous_with if present
	if alloc.ContiguousWith != nil {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("contiguous_with"), *alloc.ContiguousWith)...)
	}

	// Set metadata if present
	if len(alloc.Metadata) > 0 {
		metadataValue, diags := types.MapValueFrom(ctx, types.StringType, alloc.Metadata)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metadata"), metadataValue)...)
	}

	tflog.Info(ctx, "Imported allocation", map[string]interface{}{
		"id":        req.ID,
		"cidr":      alloc.CIDR,
		"cidr_mask": maskSize,
	})
}

// findContiguousCIDR finds a CIDR block that is immediately adjacent to the target CIDR.
func findContiguousCIDR(pool *ipam.PoolDefinition, allocs []ipam.Allocation, prefixLen int, targetCIDR string) (string, error) {
	_, targetNet, err := net.ParseCIDR(targetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid target CIDR %q: %w", targetCIDR, err)
	}

	// Calculate target range
	targetStart := ipToUint32(targetNet.IP)
	targetOnes, targetBits := targetNet.Mask.Size()
	targetSize := uint32(1) << (targetBits - targetOnes)
	targetEnd := targetStart + targetSize

	// Desired block size
	blockSize := uint32(1) << (32 - prefixLen)

	var beforeReason, afterReason string

	// Check space immediately before target
	if targetStart >= blockSize {
		beforeStart := targetStart - blockSize
		if beforeStart%blockSize == 0 {
			beforeCIDR := fmt.Sprintf("%s/%d", uint32ToIP(beforeStart), prefixLen)
			if !isInPool(pool, beforeCIDR) {
				beforeReason = fmt.Sprintf("before block %s is outside pool boundaries", beforeCIDR)
			} else if overlapsAny(beforeCIDR, allocs) {
				beforeReason = fmt.Sprintf("before block %s overlaps with existing allocation", beforeCIDR)
			} else {
				return beforeCIDR, nil
			}
		} else {
			beforeReason = fmt.Sprintf("no valid /%d boundary before target (alignment requires address divisible by %d)", prefixLen, blockSize)
		}
	} else {
		beforeReason = "target is too close to start of address space for a block before it"
	}

	// Check space immediately after target
	afterStart := targetEnd
	if afterStart%blockSize == 0 {
		afterCIDR := fmt.Sprintf("%s/%d", uint32ToIP(afterStart), prefixLen)
		if !isInPool(pool, afterCIDR) {
			afterReason = fmt.Sprintf("after block %s is outside pool boundaries", afterCIDR)
		} else if overlapsAny(afterCIDR, allocs) {
			afterReason = fmt.Sprintf("after block %s overlaps with existing allocation", afterCIDR)
		} else {
			return afterCIDR, nil
		}
	} else {
		afterReason = fmt.Sprintf("no valid /%d boundary after target (target end %s not aligned to block size %d)",
			prefixLen, uint32ToIP(afterStart), blockSize)
	}

	return "", fmt.Errorf("no contiguous /%d space available adjacent to %s: before: %s; after: %s",
		prefixLen, targetCIDR, beforeReason, afterReason)
}

func isInPool(pool *ipam.PoolDefinition, cidr string) bool {
	_, candidateNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	for _, poolCIDR := range pool.CIDR {
		_, poolNet, err := net.ParseCIDR(poolCIDR)
		if err != nil {
			continue
		}
		// Check if candidate is fully contained in pool
		candidateStart := ipToUint32(candidateNet.IP)
		candidateOnes, candidateBits := candidateNet.Mask.Size()
		candidateEnd := candidateStart + uint32(1)<<(candidateBits-candidateOnes) - 1

		poolStart := ipToUint32(poolNet.IP)
		poolOnes, poolBits := poolNet.Mask.Size()
		poolEnd := poolStart + uint32(1)<<(poolBits-poolOnes) - 1

		if candidateStart >= poolStart && candidateEnd <= poolEnd {
			return true
		}
	}
	return false
}

func uint32ToIP(n uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		(n>>24)&0xFF,
		(n>>16)&0xFF,
		(n>>8)&0xFF,
		n&0xFF)
}

func overlapsAny(cidr string, allocs []ipam.Allocation) bool {
	_, candidateNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return true // Treat errors as overlap to be safe
	}

	candidateStart := ipToUint32(candidateNet.IP)
	candidateOnes, candidateBits := candidateNet.Mask.Size()
	candidateEnd := candidateStart + uint32(1)<<(candidateBits-candidateOnes)

	for _, alloc := range allocs {
		if alloc.ParentCIDR != nil {
			continue // Skip sub-allocations
		}

		_, allocNet, err := net.ParseCIDR(alloc.CIDR)
		if err != nil {
			continue
		}

		allocStart := ipToUint32(allocNet.IP)
		allocOnes, allocBits := allocNet.Mask.Size()
		allocEnd := allocStart + uint32(1)<<(allocBits-allocOnes)

		// Check for overlap
		if candidateStart < allocEnd && candidateEnd > allocStart {
			return true
		}
	}
	return false
}

// diagnosticsToString converts diagnostics to a string for error messages.
func diagnosticsToString(diags diag.Diagnostics) string {
	var messages []string
	for _, d := range diags {
		if d.Severity() == diag.SeverityError {
			messages = append(messages, d.Summary())
		}
	}
	if len(messages) == 0 {
		return "unknown error"
	}
	return fmt.Sprintf("%v", messages)
}

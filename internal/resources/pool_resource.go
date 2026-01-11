// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"
	"fmt"
	"net"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/easytofu/terraform-provider-ipam-github/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	Name         types.String `tfsdk:"name"`
	PrivateRange types.String `tfsdk:"private_range"`
	BlockSize    types.Int64  `tfsdk:"block_size"`
	CIDR         types.String `tfsdk:"cidr"`
	Description  types.String `tfsdk:"description"`
	Metadata     types.Map    `tfsdk:"metadata"`
}

func (r *PoolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (r *PoolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an IPAM pool with automatic CIDR allocation from private IP ranges.",
		MarkdownDescription: `Manages an IPAM pool definition in pools.yaml.

Pools are automatically allocated a non-overlapping CIDR block from the specified
private IP range. The provider finds the next available block that doesn't conflict
with existing pools.

**Private Ranges (RFC 1918):**
- ` + "`10.0.0.0/8`" + ` - Class A (10.0.0.0 - 10.255.255.255)
- ` + "`172.16.0.0/12`" + ` - Class B (172.16.0.0 - 172.31.255.255)
- ` + "`192.168.0.0/16`" + ` - Class C (192.168.0.0 - 192.168.255.255)

**Example:**
` + "```hcl" + `
resource "github-ipam_pool" "stake" {
  name          = "stake"
  private_range = "10.0.0.0/8"
  block_size    = 12
  description   = "Stake product IP pool"
}

# Output: cidr = "10.0.0.0/12" (or next available)
` + "```",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "Unique name/identifier for this pool (used as pool_id in allocations).",
				MarkdownDescription: "Unique name/identifier for this pool (used as `pool_id` in allocations).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"private_range": schema.StringAttribute{
				Required:            true,
				Description:         "Private IP range to allocate from: 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16.",
				MarkdownDescription: "Private IP range to allocate from: `10.0.0.0/8`, `172.16.0.0/12`, or `192.168.0.0/16`.",
				Validators: []validator.String{
					stringvalidator.OneOf("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"block_size": schema.Int64Attribute{
				Required:            true,
				Description:         "CIDR prefix length for the pool (e.g., 12 for /12, 16 for /16).",
				MarkdownDescription: "CIDR prefix length for the pool (e.g., `12` for /12, `16` for /16).",
				Validators: []validator.Int64{
					int64validator.Between(8, 28),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"cidr": schema.StringAttribute{
				Computed:            true,
				Description:         "Allocated CIDR block (computed automatically).",
				MarkdownDescription: "Allocated CIDR block (computed automatically).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

	poolName := plan.Name.ValueString()
	privateRange := plan.PrivateRange.ValueString()
	blockSize := int(plan.BlockSize.ValueInt64())

	tflog.Debug(ctx, "Creating pool", map[string]interface{}{
		"name":          poolName,
		"private_range": privateRange,
		"block_size":    blockSize,
	})

	var allocatedCIDR string
	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		// Read pools.yaml with SHA for OCC
		pools, sha, err := r.client.GetPoolsWithSHA(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read pools: %w", err)
		}

		// Check if pool name already exists
		if _, exists := pools.GetPool(poolName); exists {
			return false, fmt.Errorf("pool with name %q already exists", poolName)
		}

		// Use private_range directly as parent CIDR
		parentCIDR := privateRange

		// Collect all existing CIDRs from all pools
		var existingCIDRs []string
		for _, pool := range pools.Pools {
			existingCIDRs = append(existingCIDRs, pool.CIDR...)
		}

		// Find next available CIDR
		newCIDR, err := findNextAvailableCIDR(parentCIDR, existingCIDRs, blockSize)
		if err != nil {
			return false, fmt.Errorf("failed to allocate CIDR from %s: %w", privateRange, err)
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
			CIDR:        []string{newCIDR},
			Description: plan.Description.ValueString(),
			Metadata:    metadata,
		}

		pools.AddPool(poolName, poolDef)

		commitMsg := fmt.Sprintf("ipam: create pool %s (%s)", poolName, newCIDR)
		err = r.client.UpdatePools(ctx, pools, sha, commitMsg)
		if r.client.IsConflictError(err) {
			tflog.Debug(ctx, "Conflict detected, will retry", map[string]interface{}{
				"attempt": attempt,
			})
			return true, err
		}

		if err == nil {
			allocatedCIDR = newCIDR
		}
		return false, err
	})

	if err != nil {
		resp.Diagnostics.AddError("Failed to create pool", err.Error())
		return
	}

	plan.CIDR = types.StringValue(allocatedCIDR)

	tflog.Info(ctx, "Created pool", map[string]interface{}{
		"name": poolName,
		"cidr": allocatedCIDR,
	})

	// Regenerate README (best effort, don't fail on error)
	if err := r.client.RegenerateREADME(ctx); err != nil {
		tflog.Warn(ctx, "Failed to regenerate README", map[string]interface{}{
			"error": err.Error(),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *PoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolName := state.Name.ValueString()

	tflog.Debug(ctx, "Reading pool", map[string]interface{}{
		"name": poolName,
	})

	pools, err := r.client.GetPools(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read pools", err.Error())
		return
	}

	poolDef, exists := pools.GetPool(poolName)
	if !exists {
		tflog.Warn(ctx, "Pool not found, removing from state", map[string]interface{}{
			"name": poolName,
		})
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state with current values
	if len(poolDef.CIDR) > 0 {
		state.CIDR = types.StringValue(poolDef.CIDR[0])
	}

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

	poolName := plan.Name.ValueString()

	tflog.Debug(ctx, "Updating pool", map[string]interface{}{
		"name": poolName,
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		pools, sha, err := r.client.GetPoolsWithSHA(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read pools: %w", err)
		}

		existingPool, exists := pools.GetPool(poolName)
		if !exists {
			return false, fmt.Errorf("pool %s not found", poolName)
		}

		// Build metadata map
		metadata := make(map[string]string)
		if !plan.Metadata.IsNull() {
			resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
			if resp.Diagnostics.HasError() {
				return false, fmt.Errorf("failed to parse metadata")
			}
		}

		// Keep existing CIDR, only update description and metadata
		poolDef := ipam.PoolDefinition{
			CIDR:        existingPool.CIDR,
			Description: plan.Description.ValueString(),
			Metadata:    metadata,
		}

		pools.AddPool(poolName, poolDef)

		commitMsg := fmt.Sprintf("ipam: update pool %s", poolName)
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
		"name": poolName,
	})

	// Regenerate README (best effort, don't fail on error)
	if err := r.client.RegenerateREADME(ctx); err != nil {
		tflog.Warn(ctx, "Failed to regenerate README", map[string]interface{}{
			"error": err.Error(),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *PoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolName := state.Name.ValueString()

	tflog.Debug(ctx, "Deleting pool", map[string]interface{}{
		"name": poolName,
	})

	retryConfig := client.NewRetryConfig(r.client.MaxRetries(), r.client.BaseDelay().Milliseconds())

	err := client.WithRetry(ctx, retryConfig, func(ctx context.Context, attempt int) (bool, error) {
		pools, sha, err := r.client.GetPoolsWithSHA(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read pools: %w", err)
		}

		if _, exists := pools.GetPool(poolName); !exists {
			// Already deleted
			tflog.Debug(ctx, "Pool already deleted", map[string]interface{}{
				"name": poolName,
			})
			return false, nil
		}

		// Check for existing allocations in this pool
		db, _, err := r.client.GetAllocations(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to read allocations: %w", err)
		}

		allocs := db.GetAllocationsForPool(poolName)
		if len(allocs) > 0 {
			return false, fmt.Errorf("cannot delete pool %s: has %d active allocations", poolName, len(allocs))
		}

		if err := pools.RemovePool(poolName); err != nil {
			return false, err
		}

		commitMsg := fmt.Sprintf("ipam: delete pool %s", poolName)
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
		"name": poolName,
	})

	// Regenerate README (best effort, don't fail on error)
	if err := r.client.RegenerateREADME(ctx); err != nil {
		tflog.Warn(ctx, "Failed to regenerate README", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

func (r *PoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	tflog.Debug(ctx, "Importing pool", map[string]interface{}{
		"name": req.ID,
	})

	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// findNextAvailableCIDR finds the next available CIDR block within the parent range
// that doesn't overlap with any existing CIDRs.
func findNextAvailableCIDR(parentCIDR string, existingCIDRs []string, prefixLen int) (string, error) {
	_, parentNet, err := net.ParseCIDR(parentCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid parent CIDR %s: %w", parentCIDR, err)
	}

	parentPrefixLen, _ := parentNet.Mask.Size()
	if prefixLen < parentPrefixLen {
		return "", fmt.Errorf("requested prefix /%d is larger than parent range /%d", prefixLen, parentPrefixLen)
	}

	// Parse existing CIDRs into networks
	var existingNets []*net.IPNet
	for _, cidr := range existingCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue // Skip invalid CIDRs
		}
		existingNets = append(existingNets, network)
	}

	// Calculate the step size for the requested prefix
	// For a /12, we need to step by 2^(32-12) = 2^20 = 1048576
	stepBits := 32 - prefixLen
	step := uint32(1) << stepBits

	// Start from the beginning of the parent range
	startIP := ipToUint32(parentNet.IP)
	endIP := startIP + (uint32(1) << (32 - parentPrefixLen)) - 1

	// Align start to the prefix boundary
	if startIP%step != 0 {
		startIP = ((startIP / step) + 1) * step
	}

	// Iterate through possible blocks
	for candidateStart := startIP; candidateStart+step-1 <= endIP; candidateStart += step {
		candidateNet := uint32ToCIDR(candidateStart, prefixLen)

		_, candidateIPNet, err := net.ParseCIDR(candidateNet)
		if err != nil {
			continue
		}

		// Check if this candidate overlaps with any existing network
		overlaps := false
		for _, existing := range existingNets {
			if networksOverlap(candidateIPNet, existing) {
				overlaps = true
				break
			}
		}

		if !overlaps {
			return candidateNet, nil
		}
	}

	return "", fmt.Errorf("no available /%d block in %s", prefixLen, parentCIDR)
}

// ipToUint32 converts a net.IP to uint32.
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// uint32ToCIDR converts a uint32 IP and prefix length to CIDR notation.
func uint32ToCIDR(ip uint32, prefixLen int) string {
	return fmt.Sprintf("%d.%d.%d.%d/%d",
		(ip>>24)&0xFF,
		(ip>>16)&0xFF,
		(ip>>8)&0xFF,
		ip&0xFF,
		prefixLen,
	)
}

// networksOverlap checks if two networks overlap.
func networksOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

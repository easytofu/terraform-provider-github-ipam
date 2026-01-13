// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/easytofu/terraform-provider-ipam-github/internal/client"
	"github.com/easytofu/terraform-provider-ipam-github/internal/datasources"
	"github.com/easytofu/terraform-provider-ipam-github/internal/resources"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure GitIPAMProvider satisfies various provider interfaces.
var _ provider.Provider = &GitIPAMProvider{}

// GitIPAMProvider defines the provider implementation.
type GitIPAMProvider struct {
	version string
}

// GitIPAMProviderModel describes the provider data model.
type GitIPAMProviderModel struct {
	Token           types.String `tfsdk:"token"`
	Owner           types.String `tfsdk:"owner"`
	Repository      types.String `tfsdk:"repository"`
	Branch          types.String `tfsdk:"branch"`
	PoolsFile       types.String `tfsdk:"pools_file"`
	AllocationsFile types.String `tfsdk:"allocations_file"`
	MaxRetries      types.Int64  `tfsdk:"max_retries"`
	BaseDelayMs     types.Int64  `tfsdk:"base_delay_ms"`
}

// New creates a new provider instance.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &GitIPAMProvider{
			version: version,
		}
	}
}

func (p *GitIPAMProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "github-ipam"
	resp.Version = p.version
}

func (p *GitIPAMProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Git-backed IPAM provider using GitHub API for optimistic concurrency control. " +
			"Uses a dual-file architecture: pools.yaml (pool definitions) and " +
			"allocations.yaml (allocation state).",
		MarkdownDescription: `Git-backed IPAM provider using GitHub API for optimistic concurrency control.

Uses a dual-file architecture:
- **pools.yaml** - Pool definitions (managed via ` + "`github-ipam_pool`" + ` resource or PR)
- **allocations.yaml** - Allocation state with optimistic concurrency control

This provider enables GitOps-native IP address management without the cost of AWS VPC IPAM.`,
		Attributes: map[string]schema.Attribute{
			"token": schema.StringAttribute{
				Description: "GitHub Personal Access Token or App Installation Token. " +
					"Can also be set via GITHUB_TOKEN environment variable.",
				MarkdownDescription: "GitHub Personal Access Token or App Installation Token. " +
					"Can also be set via `GITHUB_TOKEN` environment variable.",
				Required:  true,
				Sensitive: true,
			},
			"owner": schema.StringAttribute{
				Description:         "GitHub repository owner (user or organization).",
				MarkdownDescription: "GitHub repository owner (user or organization).",
				Required:            true,
			},
			"repository": schema.StringAttribute{
				Description:         "GitHub repository name containing IPAM data files.",
				MarkdownDescription: "GitHub repository name containing IPAM data files.",
				Required:            true,
			},
			"branch": schema.StringAttribute{
				Description:         "Git branch for IPAM data. Defaults to 'main'.",
				MarkdownDescription: "Git branch for IPAM data. Defaults to `main`.",
				Optional:            true,
			},
			"pools_file": schema.StringAttribute{
				Description: "Path to pools.yaml in repository. Defaults to 'network/pools.yaml'. " +
					"This file is read-only by the provider; pool definitions are managed via PR.",
				MarkdownDescription: "Path to pools.yaml in repository. Defaults to `network/pools.yaml`. " +
					"This file is read-only by the provider; pool definitions are managed via PR.",
				Optional: true,
			},
			"allocations_file": schema.StringAttribute{
				Description: "Path to allocations.yaml in repository. Defaults to 'network/allocations.yaml'. " +
					"This file is read-write by the provider with optimistic concurrency control.",
				MarkdownDescription: "Path to allocations.yaml in repository. Defaults to `network/allocations.yaml`. " +
					"This file is read-write by the provider with optimistic concurrency control.",
				Optional: true,
			},
			"max_retries": schema.Int64Attribute{
				Description:         "Maximum retry attempts on conflict. Defaults to 10.",
				MarkdownDescription: "Maximum retry attempts on conflict. Defaults to `10`.",
				Optional:            true,
			},
			"base_delay_ms": schema.Int64Attribute{
				Description:         "Base delay in milliseconds for exponential backoff. Defaults to 200.",
				MarkdownDescription: "Base delay in milliseconds for exponential backoff. Defaults to `200`.",
				Optional:            true,
			},
		},
	}
}

func (p *GitIPAMProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config GitIPAMProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values
	branch := "main"
	if !config.Branch.IsNull() {
		branch = config.Branch.ValueString()
	}

	poolsFile := "network/pools.yaml"
	if !config.PoolsFile.IsNull() {
		poolsFile = config.PoolsFile.ValueString()
	}

	allocationsFile := "network/allocations.yaml"
	if !config.AllocationsFile.IsNull() {
		allocationsFile = config.AllocationsFile.ValueString()
	}

	maxRetries := int64(10)
	if !config.MaxRetries.IsNull() {
		maxRetries = config.MaxRetries.ValueInt64()
	}

	baseDelayMs := int64(200)
	if !config.BaseDelayMs.IsNull() {
		baseDelayMs = config.BaseDelayMs.ValueInt64()
	}

	// Create GitHub client
	ghClient := client.NewGitHubClient(
		config.Token.ValueString(),
		config.Owner.ValueString(),
		config.Repository.ValueString(),
		branch,
		poolsFile,
		allocationsFile,
		int(maxRetries),
		baseDelayMs,
	)

	// Make the client available to resources and data sources
	resp.DataSourceData = ghClient
	resp.ResourceData = ghClient
}

func (p *GitIPAMProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewAllocationResource,
		resources.NewPoolResource,
	}
}

func (p *GitIPAMProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasources.NewPoolsDataSource,
		datasources.NewPoolDataSource,
		datasources.NewAllocationDataSource,
		datasources.NewAllocationsDataSource,
		datasources.NewNextAvailableDataSource,
	}
}

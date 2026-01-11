// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/easytofu/terraform-provider-ipam-github/internal/ipam"
	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

// GitHubClient wraps the GitHub API client for IPAM operations.
type GitHubClient struct {
	client          *github.Client
	owner           string
	repo            string
	branch          string
	poolsFile       string // Read-only: pools.yaml
	allocationsFile string // Read-write: allocations.json
	maxRetries      int
	baseDelay       time.Duration
}

// NewGitHubClient creates a new GitHub client for IPAM operations.
func NewGitHubClient(token, owner, repo, branch, poolsFile, allocationsFile string, maxRetries int, baseDelayMs int64) *GitHubClient {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	ghClient := github.NewClient(tc)

	return &GitHubClient{
		client:          ghClient,
		owner:           owner,
		repo:            repo,
		branch:          branch,
		poolsFile:       poolsFile,
		allocationsFile: allocationsFile,
		maxRetries:      maxRetries,
		baseDelay:       time.Duration(baseDelayMs) * time.Millisecond,
	}
}

// GetPools reads pools.yaml. If the file doesn't exist, it creates an empty one.
func (c *GitHubClient) GetPools(ctx context.Context) (*ipam.PoolsConfig, error) {
	fileContent, _, resp, err := c.client.Repositories.GetContents(
		ctx,
		c.owner,
		c.repo,
		c.poolsFile,
		&github.RepositoryContentGetOptions{Ref: c.branch},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			// File doesn't exist, create it with empty pools
			emptyPools := ipam.NewPoolsConfig()
			if createErr := c.createPoolsFile(ctx, emptyPools); createErr != nil {
				return nil, fmt.Errorf("pools file not found and failed to create: %w", createErr)
			}
			return emptyPools, nil
		}
		return nil, fmt.Errorf("failed to get pools file: %w", err)
	}

	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pools content: %w", err)
	}

	var pools ipam.PoolsConfig
	if err := yaml.Unmarshal(content, &pools); err != nil {
		return nil, fmt.Errorf("failed to parse pools YAML: %w", err)
	}

	// Ensure Pools map is initialized
	if pools.Pools == nil {
		pools.Pools = make(map[string]ipam.PoolDefinition)
	}

	return &pools, nil
}

// createPoolsFile creates the pools.yaml file with initial content.
func (c *GitHubClient) createPoolsFile(ctx context.Context, pools *ipam.PoolsConfig) error {
	content, err := yaml.Marshal(pools)
	if err != nil {
		return fmt.Errorf("failed to serialize pools: %w", err)
	}

	// Add a header comment to the YAML
	header := []byte("# IPAM Pool Definitions\n# Define your IP address pools here.\n# Example:\n# pools:\n#   my-pool:\n#     cidr:\n#       - \"10.0.0.0/8\"\n#     description: \"My IP pool\"\n#     metadata:\n#       environment: \"production\"\n\n")
	content = append(header, content...)

	opts := &github.RepositoryContentFileOptions{
		Message: github.String("Initialize IPAM pools configuration"),
		Content: content,
		Branch:  github.String(c.branch),
	}

	_, _, err = c.client.Repositories.CreateFile(ctx, c.owner, c.repo, c.poolsFile, opts)
	if err != nil {
		return fmt.Errorf("failed to create pools file: %w", err)
	}

	return nil
}

// GetAllocations reads allocations.json with SHA for OCC.
func (c *GitHubClient) GetAllocations(ctx context.Context) (*ipam.AllocationsDatabase, string, error) {
	fileContent, _, resp, err := c.client.Repositories.GetContents(
		ctx,
		c.owner,
		c.repo,
		c.allocationsFile,
		&github.RepositoryContentGetOptions{Ref: c.branch},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			// File doesn't exist, return empty database with empty SHA
			return ipam.NewAllocationsDatabase(), "", nil
		}
		return nil, "", fmt.Errorf("failed to get allocations file: %w", err)
	}

	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode allocations content: %w", err)
	}

	var db ipam.AllocationsDatabase
	if err := json.Unmarshal(content, &db); err != nil {
		return nil, "", fmt.Errorf("failed to parse allocations JSON: %w", err)
	}

	return &db, *fileContent.SHA, nil
}

// UpdateAllocations writes allocations.json with OCC via SHA.
func (c *GitHubClient) UpdateAllocations(ctx context.Context, db *ipam.AllocationsDatabase, sha, commitMessage string) error {
	content, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize allocations: %w", err)
	}

	opts := &github.RepositoryContentFileOptions{
		Message: github.String(commitMessage),
		Content: content,
		Branch:  github.String(c.branch),
	}

	if sha != "" {
		opts.SHA = github.String(sha)
	}

	_, _, err = c.client.Repositories.UpdateFile(ctx, c.owner, c.repo, c.allocationsFile, opts)
	return err
}

// IsConflictError checks if an error is a 409 Conflict from the GitHub API.
func (c *GitHubClient) IsConflictError(err error) bool {
	if err == nil {
		return false
	}
	if ghErr, ok := err.(*github.ErrorResponse); ok {
		return ghErr.Response.StatusCode == 409
	}
	return false
}

// MaxRetries returns the configured maximum retry attempts.
func (c *GitHubClient) MaxRetries() int {
	return c.maxRetries
}

// BaseDelay returns the configured base delay for backoff.
func (c *GitHubClient) BaseDelay() time.Duration {
	return c.baseDelay
}

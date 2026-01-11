// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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
			// File doesn't exist, try to create it with empty pools
			emptyPools := ipam.NewPoolsConfig()
			if createErr := c.createPoolsFile(ctx, emptyPools); createErr != nil {
				// If we get a 409 conflict, another process created the file - retry read
				if c.IsConflictError(createErr) {
					return c.GetPools(ctx)
				}
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
	// Return raw error to preserve type for IsConflictError detection
	return err
}

// GetPoolsWithSHA reads pools.yaml and returns the SHA for OCC updates.
// If the file doesn't exist, it creates an empty one and returns the new SHA.
func (c *GitHubClient) GetPoolsWithSHA(ctx context.Context) (*ipam.PoolsConfig, string, error) {
	fileContent, _, resp, err := c.client.Repositories.GetContents(
		ctx,
		c.owner,
		c.repo,
		c.poolsFile,
		&github.RepositoryContentGetOptions{Ref: c.branch},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			// File doesn't exist, try to create it with empty pools
			emptyPools := ipam.NewPoolsConfig()
			if createErr := c.createPoolsFile(ctx, emptyPools); createErr != nil {
				// If we get a 409 conflict, another process created the file - retry read
				if c.IsConflictError(createErr) {
					return c.GetPoolsWithSHA(ctx)
				}
				return nil, "", fmt.Errorf("pools file not found and failed to create: %w", createErr)
			}
			// Fetch again to get the SHA
			return c.GetPoolsWithSHA(ctx)
		}
		return nil, "", fmt.Errorf("failed to get pools file: %w", err)
	}

	content, err := base64.StdEncoding.DecodeString(*fileContent.Content)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode pools content: %w", err)
	}

	var pools ipam.PoolsConfig
	if err := yaml.Unmarshal(content, &pools); err != nil {
		return nil, "", fmt.Errorf("failed to parse pools YAML: %w", err)
	}

	// Ensure Pools map is initialized
	if pools.Pools == nil {
		pools.Pools = make(map[string]ipam.PoolDefinition)
	}

	return &pools, *fileContent.SHA, nil
}

// UpdatePools writes pools.yaml with OCC via SHA.
func (c *GitHubClient) UpdatePools(ctx context.Context, pools *ipam.PoolsConfig, sha, commitMessage string) error {
	content, err := yaml.Marshal(pools)
	if err != nil {
		return fmt.Errorf("failed to serialize pools: %w", err)
	}

	opts := &github.RepositoryContentFileOptions{
		Message: github.String(commitMessage),
		Content: content,
		Branch:  github.String(c.branch),
	}

	if sha != "" {
		opts.SHA = github.String(sha)
	}

	_, _, err = c.client.Repositories.UpdateFile(ctx, c.owner, c.repo, c.poolsFile, opts)
	return err
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
// If SHA is empty (file doesn't exist), creates the file.
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
		// Update existing file with OCC
		opts.SHA = github.String(sha)
		_, _, err = c.client.Repositories.UpdateFile(ctx, c.owner, c.repo, c.allocationsFile, opts)
	} else {
		// Create new file
		_, _, err = c.client.Repositories.CreateFile(ctx, c.owner, c.repo, c.allocationsFile, opts)
	}
	return err
}

// IsConflictError checks if an error is a 409 Conflict from the GitHub API.
func (c *GitHubClient) IsConflictError(err error) bool {
	if err == nil {
		return false
	}
	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		return ghErr.Response != nil && ghErr.Response.StatusCode == 409
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

// UpdateREADME updates the .github/README.md file with current IPAM status.
func (c *GitHubClient) UpdateREADME(ctx context.Context, content string) error {
	readmePath := ".github/README.md"

	// Try to get existing file for SHA
	fileContent, _, resp, err := c.client.Repositories.GetContents(
		ctx,
		c.owner,
		c.repo,
		readmePath,
		&github.RepositoryContentGetOptions{Ref: c.branch},
	)

	opts := &github.RepositoryContentFileOptions{
		Message: github.String("docs: update IPAM status"),
		Content: []byte(content),
		Branch:  github.String(c.branch),
	}

	if err == nil && fileContent != nil {
		// File exists, update it
		opts.SHA = github.String(*fileContent.SHA)
		_, _, err = c.client.Repositories.UpdateFile(ctx, c.owner, c.repo, readmePath, opts)
	} else if resp != nil && resp.StatusCode == 404 {
		// File doesn't exist, create it
		_, _, err = c.client.Repositories.CreateFile(ctx, c.owner, c.repo, readmePath, opts)
	} else {
		return fmt.Errorf("failed to check README existence: %w", err)
	}

	return err
}

// RegenerateREADME regenerates all IPAM documentation files.
func (c *GitHubClient) RegenerateREADME(ctx context.Context) error {
	pools, err := c.GetPools(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pools for README: %w", err)
	}

	allocations, _, err := c.GetAllocations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get allocations for README: %w", err)
	}

	// Generate all files
	files := ipam.GenerateAllFiles(pools, allocations)

	// Write each file
	for path, content := range files.Files {
		if err := c.writeFile(ctx, path, content); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	return nil
}

// writeFile writes or updates a file in the repository.
func (c *GitHubClient) writeFile(ctx context.Context, path, content string) error {
	// Try to get existing file for SHA
	fileContent, _, resp, err := c.client.Repositories.GetContents(
		ctx,
		c.owner,
		c.repo,
		path,
		&github.RepositoryContentGetOptions{Ref: c.branch},
	)

	opts := &github.RepositoryContentFileOptions{
		Message: github.String(fmt.Sprintf("docs: update %s", path)),
		Content: []byte(content),
		Branch:  github.String(c.branch),
	}

	if err == nil && fileContent != nil {
		// File exists, update it
		opts.SHA = github.String(*fileContent.SHA)
		_, _, err = c.client.Repositories.UpdateFile(ctx, c.owner, c.repo, path, opts)
	} else if resp != nil && resp.StatusCode == 404 {
		// File doesn't exist, create it
		_, _, err = c.client.Repositories.CreateFile(ctx, c.owner, c.repo, path, opts)
	} else if err != nil {
		return fmt.Errorf("failed to check file existence: %w", err)
	}

	return err
}

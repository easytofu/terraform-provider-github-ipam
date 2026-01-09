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

// GetPools reads pools.yaml (read-only, no SHA needed for OCC).
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
			return nil, fmt.Errorf("pools file not found: %s", c.poolsFile)
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

	return &pools, nil
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

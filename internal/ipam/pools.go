// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"fmt"
	"net"
)

// PoolsConfig represents the pools.yaml file structure.
// This file is READ-ONLY by the provider; changes require PR review.
type PoolsConfig struct {
	Pools map[string]PoolDefinition `yaml:"pools"`
}

// NewPoolsConfig creates a new empty pools configuration.
func NewPoolsConfig() *PoolsConfig {
	return &PoolsConfig{
		Pools: make(map[string]PoolDefinition),
	}
}

// PoolDefinition defines a pool in pools.yaml.
type PoolDefinition struct {
	CIDR        []string          `yaml:"cidr"`        // Array of CIDRs for this pool
	Description string            `yaml:"description"` // Human-readable description
	Metadata    map[string]string `yaml:"metadata"`    // Arbitrary key-value metadata
}

// GetPool looks up a pool by pool_id.
func (p *PoolsConfig) GetPool(poolID string) (*PoolDefinition, bool) {
	if p.Pools == nil {
		return nil, false
	}
	pool, exists := p.Pools[poolID]
	if !exists {
		return nil, false
	}
	return &pool, true
}

// ListPoolIDs returns all pool IDs.
func (p *PoolsConfig) ListPoolIDs() []string {
	if p.Pools == nil {
		return nil
	}
	ids := make([]string, 0, len(p.Pools))
	for id := range p.Pools {
		ids = append(ids, id)
	}
	return ids
}

// ValidatePools ensures all pools have valid CIDRs and no overlaps.
func (p *PoolsConfig) ValidatePools() error {
	if p.Pools == nil {
		return nil
	}

	allNetworks := make([]*net.IPNet, 0)

	for poolID, pool := range p.Pools {
		if len(pool.CIDR) == 0 {
			return fmt.Errorf("pool %s has no CIDRs defined", poolID)
		}

		for _, cidrStr := range pool.CIDR {
			_, network, err := net.ParseCIDR(cidrStr)
			if err != nil {
				return fmt.Errorf("pool %s has invalid CIDR %s: %w", poolID, cidrStr, err)
			}

			// Check for overlaps with other pools
			for _, existing := range allNetworks {
				if networksOverlap(network, existing) {
					return fmt.Errorf("pool %s CIDR %s overlaps with another pool", poolID, cidrStr)
				}
			}
			allNetworks = append(allNetworks, network)
		}
	}

	return nil
}

// networksOverlap checks if two networks overlap.
func networksOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

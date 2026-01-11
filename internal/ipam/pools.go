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
	CIDR        []string          `yaml:"cidr"`                  // Array of CIDRs for this pool
	Description string            `yaml:"description"`           // Human-readable description
	Metadata    map[string]string `yaml:"metadata"`              // Arbitrary key-value metadata
	Reserved    bool              `yaml:"reserved,omitempty"`    // If true, pool is reserved (no allocations allowed)
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

// AddPool adds or updates a pool definition.
func (p *PoolsConfig) AddPool(poolID string, pool PoolDefinition) {
	if p.Pools == nil {
		p.Pools = make(map[string]PoolDefinition)
	}
	p.Pools[poolID] = pool
}

// RemovePool removes a pool by ID. Returns error if pool doesn't exist.
func (p *PoolsConfig) RemovePool(poolID string) error {
	if p.Pools == nil {
		return fmt.Errorf("pool %s not found", poolID)
	}
	if _, exists := p.Pools[poolID]; !exists {
		return fmt.Errorf("pool %s not found", poolID)
	}
	delete(p.Pools, poolID)
	return nil
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
// Two networks overlap if: startA <= endB AND startB <= endA
func networksOverlap(a, b *net.IPNet) bool {
	// Get start and end of network A
	aStart := ipToUint32(a.IP)
	aOnes, aBits := a.Mask.Size()
	aEnd := aStart + (uint32(1) << (aBits - aOnes)) - 1

	// Get start and end of network B
	bStart := ipToUint32(b.IP)
	bOnes, bBits := b.Mask.Size()
	bEnd := bStart + (uint32(1) << (bBits - bOnes)) - 1

	// Two ranges overlap if startA <= endB AND startB <= endA
	return aStart <= bEnd && bStart <= aEnd
}

// ipToUint32 converts a net.IP to uint32 for calculations.
func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

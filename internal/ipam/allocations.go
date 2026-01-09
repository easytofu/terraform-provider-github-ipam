// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"fmt"
	"time"
)

// AllocationsDatabase represents the allocations.json file structure.
// This file is READ-WRITE by the provider with OCC via GitHub SHA.
type AllocationsDatabase struct {
	Version     string                  `json:"version"`
	Allocations map[string][]Allocation `json:"allocations"` // pool_id -> allocations
}

// Allocation represents a single CIDR allocation.
type Allocation struct {
	CIDR       string            `json:"cidr"`
	ID         string            `json:"id"`                    // UUID linking to Terraform state
	Name       string            `json:"name,omitempty"`        // Human-readable name
	ParentCIDR *string           `json:"parent_cidr,omitempty"` // For sub-allocations
	Metadata   map[string]string `json:"metadata,omitempty"`    // Arbitrary key-value metadata
	CreatedAt  string            `json:"created_at,omitempty"`  // RFC3339 timestamp
}

// NewAllocationsDatabase creates a new empty allocations database.
func NewAllocationsDatabase() *AllocationsDatabase {
	return &AllocationsDatabase{
		Version:     "1.0",
		Allocations: make(map[string][]Allocation),
	}
}

// FindAllocationByID searches all pools for an allocation by ID.
func (d *AllocationsDatabase) FindAllocationByID(id string) (*Allocation, string, bool) {
	for poolID, allocations := range d.Allocations {
		for i := range allocations {
			if allocations[i].ID == id {
				return &allocations[i], poolID, true
			}
		}
	}
	return nil, "", false
}

// FindAllocationByCIDR searches all pools for an allocation by CIDR.
func (d *AllocationsDatabase) FindAllocationByCIDR(cidr string) (*Allocation, string, bool) {
	for poolID, allocations := range d.Allocations {
		for i := range allocations {
			if allocations[i].CIDR == cidr {
				return &allocations[i], poolID, true
			}
		}
	}
	return nil, "", false
}

// GetAllocationsForPool returns all allocations for a pool.
func (d *AllocationsDatabase) GetAllocationsForPool(poolID string) []Allocation {
	if d.Allocations == nil {
		return nil
	}
	return d.Allocations[poolID]
}

// GetAllocationsForParent returns allocations with a specific parent_cidr.
func (d *AllocationsDatabase) GetAllocationsForParent(parentCIDR string) []Allocation {
	var result []Allocation
	for _, allocations := range d.Allocations {
		for _, alloc := range allocations {
			if alloc.ParentCIDR != nil && *alloc.ParentCIDR == parentCIDR {
				result = append(result, alloc)
			}
		}
	}
	return result
}

// AddAllocation adds an allocation to a pool.
func (d *AllocationsDatabase) AddAllocation(poolID string, alloc Allocation) {
	if d.Allocations == nil {
		d.Allocations = make(map[string][]Allocation)
	}
	alloc.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	d.Allocations[poolID] = append(d.Allocations[poolID], alloc)
}

// RemoveAllocation removes an allocation by ID.
func (d *AllocationsDatabase) RemoveAllocation(poolID, id string) error {
	allocations, exists := d.Allocations[poolID]
	if !exists {
		return fmt.Errorf("pool %s has no allocations", poolID)
	}

	newAllocations := make([]Allocation, 0, len(allocations))
	found := false
	for _, alloc := range allocations {
		if alloc.ID == id {
			found = true
			continue
		}
		newAllocations = append(newAllocations, alloc)
	}

	if !found {
		return fmt.Errorf("allocation %s not found in pool %s", id, poolID)
	}

	d.Allocations[poolID] = newAllocations
	return nil
}

// AllAllocations returns a flat list of all allocations across all pools.
func (d *AllocationsDatabase) AllAllocations() []Allocation {
	var result []Allocation
	for _, allocations := range d.Allocations {
		result = append(result, allocations...)
	}
	return result
}

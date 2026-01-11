// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"testing"
	"time"
)

func TestNewAllocationsDatabase(t *testing.T) {
	db := NewAllocationsDatabase()
	if db == nil {
		t.Fatal("NewAllocationsDatabase returned nil")
		return
	}
	if db.Version != "1.0" {
		t.Errorf("expected version '1.0', got '%s'", db.Version)
	}
	if db.Allocations == nil {
		t.Error("Allocations map should be initialized")
	}
}

func TestAllocationsDatabase_AddAllocation_NewPool(t *testing.T) {
	db := NewAllocationsDatabase()

	alloc := Allocation{
		CIDR: "10.0.0.0/24",
		ID:   "test-id",
		Name: "test-alloc",
	}

	db.AddAllocation("test-pool", alloc)

	if len(db.Allocations) != 1 {
		t.Errorf("expected 1 pool in allocations, got %d", len(db.Allocations))
	}

	poolAllocs := db.Allocations["test-pool"]
	if len(poolAllocs) != 1 {
		t.Errorf("expected 1 allocation in pool, got %d", len(poolAllocs))
	}

	if poolAllocs[0].CIDR != "10.0.0.0/24" {
		t.Errorf("CIDR mismatch: got %s", poolAllocs[0].CIDR)
	}
}

func TestAllocationsDatabase_AddAllocation_ExistingPool(t *testing.T) {
	db := NewAllocationsDatabase()

	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "first"})
	db.AddAllocation("pool", Allocation{CIDR: "10.0.1.0/24", ID: "id-2", Name: "second"})

	poolAllocs := db.Allocations["pool"]
	if len(poolAllocs) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(poolAllocs))
	}
}

func TestAllocationsDatabase_AddAllocation_SetsCreatedAt(t *testing.T) {
	db := NewAllocationsDatabase()

	before := time.Now().UTC().Add(-1 * time.Second)
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "test"})
	after := time.Now().UTC().Add(1 * time.Second)

	alloc := db.Allocations["pool"][0]
	if alloc.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}

	createdAt, err := time.Parse(time.RFC3339, alloc.CreatedAt)
	if err != nil {
		t.Errorf("CreatedAt should be valid RFC3339: %v", err)
	}

	if createdAt.Before(before) || createdAt.After(after) {
		t.Errorf("CreatedAt %v should be between %v and %v", createdAt, before, after)
	}
}

func TestAllocationsDatabase_AddAllocation_OverwritesCreatedAt(t *testing.T) {
	// Note: AddAllocation always sets CreatedAt to current time,
	// regardless of what value was passed in. This documents the actual behavior.
	db := NewAllocationsDatabase()

	originalTime := "2024-01-15T10:30:00Z"
	db.AddAllocation("pool", Allocation{
		CIDR:      "10.0.0.0/24",
		ID:        "id-1",
		Name:      "test",
		CreatedAt: originalTime,
	})

	alloc := db.Allocations["pool"][0]
	// CreatedAt is always overwritten with current time
	if alloc.CreatedAt == originalTime {
		t.Error("CreatedAt should be overwritten with current time, but original was kept")
	}
	if alloc.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}
}

func TestAllocationsDatabase_AddAllocation_NilAllocations(t *testing.T) {
	db := &AllocationsDatabase{
		Version:     "1.0",
		Allocations: nil,
	}

	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "test"})

	if db.Allocations == nil {
		t.Error("Allocations should be initialized")
	}
	if len(db.Allocations["pool"]) != 1 {
		t.Error("allocation should be added")
	}
}

func TestAllocationsDatabase_FindAllocationByID_Found(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool-a", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "first"})
	db.AddAllocation("pool-a", Allocation{CIDR: "10.0.1.0/24", ID: "id-2", Name: "second"})
	db.AddAllocation("pool-b", Allocation{CIDR: "10.1.0.0/24", ID: "id-3", Name: "third"})

	alloc, poolID, found := db.FindAllocationByID("id-2")
	if !found {
		t.Fatal("allocation should be found")
	}
	if poolID != "pool-a" {
		t.Errorf("expected pool-a, got %s", poolID)
	}
	if alloc.Name != "second" {
		t.Errorf("expected 'second', got '%s'", alloc.Name)
	}
}

func TestAllocationsDatabase_FindAllocationByID_NotFound(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "test"})

	_, _, found := db.FindAllocationByID("missing")
	if found {
		t.Error("allocation should not be found")
	}
}

func TestAllocationsDatabase_FindAllocationByID_EmptyDB(t *testing.T) {
	db := NewAllocationsDatabase()

	_, _, found := db.FindAllocationByID("any")
	if found {
		t.Error("allocation should not be found in empty DB")
	}
}

func TestAllocationsDatabase_FindAllocationByCIDR_Found(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "target"})
	db.AddAllocation("pool", Allocation{CIDR: "10.0.1.0/24", ID: "id-2", Name: "other"})

	alloc, poolID, found := db.FindAllocationByCIDR("10.0.0.0/24")
	if !found {
		t.Fatal("allocation should be found")
	}
	if poolID != "pool" {
		t.Errorf("expected 'pool', got '%s'", poolID)
	}
	if alloc.Name != "target" {
		t.Errorf("expected 'target', got '%s'", alloc.Name)
	}
}

func TestAllocationsDatabase_FindAllocationByCIDR_NotFound(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "test"})

	_, _, found := db.FindAllocationByCIDR("10.1.0.0/24")
	if found {
		t.Error("allocation should not be found")
	}
}

func TestAllocationsDatabase_FindAllocationByName_Found(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool-a", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "vpc-prod"})
	db.AddAllocation("pool-b", Allocation{CIDR: "10.1.0.0/24", ID: "id-2", Name: "vpc-dev"})

	alloc, poolID, found := db.FindAllocationByName("vpc-dev")
	if !found {
		t.Fatal("allocation should be found")
	}
	if poolID != "pool-b" {
		t.Errorf("expected 'pool-b', got '%s'", poolID)
	}
	if alloc.ID != "id-2" {
		t.Errorf("expected 'id-2', got '%s'", alloc.ID)
	}
}

func TestAllocationsDatabase_FindAllocationByName_NotFound(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "existing"})

	_, _, found := db.FindAllocationByName("missing")
	if found {
		t.Error("allocation should not be found")
	}
}

func TestAllocationsDatabase_GetAllocationsForPool_Found(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("target-pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "first"})
	db.AddAllocation("target-pool", Allocation{CIDR: "10.0.1.0/24", ID: "id-2", Name: "second"})
	db.AddAllocation("other-pool", Allocation{CIDR: "10.1.0.0/24", ID: "id-3", Name: "other"})

	allocs := db.GetAllocationsForPool("target-pool")
	if len(allocs) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(allocs))
	}
}

func TestAllocationsDatabase_GetAllocationsForPool_Empty(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("other-pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "other"})

	allocs := db.GetAllocationsForPool("empty-pool")
	// Note: Returns nil for non-existent pools (standard Go map behavior)
	if len(allocs) != 0 {
		t.Errorf("expected 0 allocations, got %d", len(allocs))
	}
}

func TestAllocationsDatabase_GetAllocationsForParent_Found(t *testing.T) {
	db := NewAllocationsDatabase()
	parentCIDR := "10.0.0.0/16"

	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/16", ID: "parent-id", Name: "parent"})
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "child-1", Name: "child1", ParentCIDR: &parentCIDR})
	db.AddAllocation("pool", Allocation{CIDR: "10.0.1.0/24", ID: "child-2", Name: "child2", ParentCIDR: &parentCIDR})
	db.AddAllocation("pool", Allocation{CIDR: "10.1.0.0/24", ID: "other", Name: "other"}) // No parent

	children := db.GetAllocationsForParent(parentCIDR)
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestAllocationsDatabase_GetAllocationsForParent_NoChildren(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/16", ID: "parent-id", Name: "parent"})

	children := db.GetAllocationsForParent("10.0.0.0/16")
	if len(children) != 0 {
		t.Errorf("expected 0 children, got %d", len(children))
	}
}

func TestAllocationsDatabase_RemoveAllocation_Success(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "first"})
	db.AddAllocation("pool", Allocation{CIDR: "10.0.1.0/24", ID: "id-2", Name: "second"})

	err := db.RemoveAllocation("pool", "id-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	allocs := db.GetAllocationsForPool("pool")
	if len(allocs) != 1 {
		t.Errorf("expected 1 allocation after removal, got %d", len(allocs))
	}
	if allocs[0].ID != "id-2" {
		t.Error("wrong allocation removed")
	}
}

func TestAllocationsDatabase_RemoveAllocation_PoolNotFound(t *testing.T) {
	db := NewAllocationsDatabase()

	err := db.RemoveAllocation("missing-pool", "any-id")
	if err == nil {
		t.Error("expected error for missing pool")
	}
}

func TestAllocationsDatabase_RemoveAllocation_AllocationNotFound(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "test"})

	err := db.RemoveAllocation("pool", "missing-id")
	if err == nil {
		t.Error("expected error for missing allocation")
	}
}

func TestAllocationsDatabase_RemoveAllocation_NilAllocations(t *testing.T) {
	db := &AllocationsDatabase{Allocations: nil}

	err := db.RemoveAllocation("pool", "id")
	if err == nil {
		t.Error("expected error for nil allocations")
	}
}

func TestAllocationsDatabase_AllAllocations_Empty(t *testing.T) {
	db := NewAllocationsDatabase()

	all := db.AllAllocations()
	// Note: Returns nil when no allocations exist
	if len(all) != 0 {
		t.Errorf("expected 0 allocations, got %d", len(all))
	}
}

func TestAllocationsDatabase_AllAllocations_MultiplePools(t *testing.T) {
	db := NewAllocationsDatabase()
	db.AddAllocation("pool-a", Allocation{CIDR: "10.0.0.0/24", ID: "id-1", Name: "a1"})
	db.AddAllocation("pool-a", Allocation{CIDR: "10.0.1.0/24", ID: "id-2", Name: "a2"})
	db.AddAllocation("pool-b", Allocation{CIDR: "10.1.0.0/24", ID: "id-3", Name: "b1"})

	all := db.AllAllocations()
	if len(all) != 3 {
		t.Errorf("expected 3 allocations, got %d", len(all))
	}
}

func TestAllocation_Reserved(t *testing.T) {
	alloc := Allocation{
		CIDR:     "10.0.0.0/24",
		ID:       "id",
		Name:     "reserved-block",
		Reserved: true,
	}

	if !alloc.Reserved {
		t.Error("allocation should be reserved")
	}
}

func TestAllocation_ParentCIDR(t *testing.T) {
	parentCIDR := "10.0.0.0/16"
	alloc := Allocation{
		CIDR:       "10.0.0.0/24",
		ID:         "id",
		Name:       "child",
		ParentCIDR: &parentCIDR,
	}

	if alloc.ParentCIDR == nil {
		t.Fatal("ParentCIDR should not be nil")
	}
	if *alloc.ParentCIDR != "10.0.0.0/16" {
		t.Errorf("ParentCIDR mismatch: got %s", *alloc.ParentCIDR)
	}
}

func TestAllocation_ContiguousWith(t *testing.T) {
	contiguous := "10.0.0.0/24"
	alloc := Allocation{
		CIDR:           "10.0.1.0/24",
		ID:             "id",
		Name:           "adjacent",
		ContiguousWith: &contiguous,
	}

	if alloc.ContiguousWith == nil {
		t.Fatal("ContiguousWith should not be nil")
	}
	if *alloc.ContiguousWith != "10.0.0.0/24" {
		t.Errorf("ContiguousWith mismatch: got %s", *alloc.ContiguousWith)
	}
}

func TestAllocation_Metadata(t *testing.T) {
	alloc := Allocation{
		CIDR: "10.0.0.0/24",
		ID:   "id",
		Name: "test",
		Metadata: map[string]string{
			"env":    "production",
			"region": "us-east-1",
		},
	}

	if alloc.Metadata["env"] != "production" {
		t.Error("metadata env mismatch")
	}
	if alloc.Metadata["region"] != "us-east-1" {
		t.Error("metadata region mismatch")
	}
}

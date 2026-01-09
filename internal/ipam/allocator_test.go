// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"testing"
)

func TestFindNextAvailableInPool_EmptyPool(t *testing.T) {
	allocator := NewAllocator()
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/8"},
	}

	cidr, err := allocator.FindNextAvailableInPool(poolDef, []Allocation{}, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.0.0/24" {
		t.Errorf("expected 10.0.0.0/24, got %s", cidr)
	}
}

func TestFindNextAvailableInPool_FirstTaken(t *testing.T) {
	allocator := NewAllocator()
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/8"},
	}

	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1"},
	}

	cidr, err := allocator.FindNextAvailableInPool(poolDef, existing, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.1.0/24" {
		t.Errorf("expected 10.0.1.0/24, got %s", cidr)
	}
}

func TestFindNextAvailableInPool_FindsGap(t *testing.T) {
	allocator := NewAllocator()
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/8"},
	}

	// Gap at 10.0.1.0/24
	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1"},
		{CIDR: "10.0.2.0/24", ID: "test-2"},
	}

	cidr, err := allocator.FindNextAvailableInPool(poolDef, existing, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.1.0/24" {
		t.Errorf("expected 10.0.1.0/24, got %s", cidr)
	}
}

func TestFindNextAvailableInPool_PoolExhausted(t *testing.T) {
	allocator := NewAllocator()
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/24"},
	}

	// Fill the /24 with two /25s
	existing := []Allocation{
		{CIDR: "10.0.0.0/25", ID: "test-1"},
		{CIDR: "10.0.0.128/25", ID: "test-2"},
	}

	_, err := allocator.FindNextAvailableInPool(poolDef, existing, 25)
	if err == nil {
		t.Error("expected error for exhausted pool")
	}
}

func TestFindNextAvailableInPool_LargerThanPool(t *testing.T) {
	allocator := NewAllocator()
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/24"},
	}

	// Try to allocate a /16 from a /24 pool
	_, err := allocator.FindNextAvailableInPool(poolDef, []Allocation{}, 16)
	if err == nil {
		t.Error("expected error for prefix larger than pool")
	}
}

func TestFindNextAvailableInParent_EmptyParent(t *testing.T) {
	allocator := NewAllocator()

	cidr, err := allocator.FindNextAvailableInParent("10.0.0.0/16", []Allocation{}, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.0.0/24" {
		t.Errorf("expected 10.0.0.0/24, got %s", cidr)
	}
}

func TestFindNextAvailableInParent_WithExisting(t *testing.T) {
	allocator := NewAllocator()

	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1", ParentCIDR: strPtr("10.0.0.0/16")},
		{CIDR: "10.0.1.0/24", ID: "test-2", ParentCIDR: strPtr("10.0.0.0/16")},
	}

	cidr, err := allocator.FindNextAvailableInParent("10.0.0.0/16", existing, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.2.0/24" {
		t.Errorf("expected 10.0.2.0/24, got %s", cidr)
	}
}

func TestFindNextAvailableInParent_DifferentSizes(t *testing.T) {
	allocator := NewAllocator()

	// Allocate a /20 first, then try to get another /20
	existing := []Allocation{
		{CIDR: "10.0.0.0/20", ID: "test-1", ParentCIDR: strPtr("10.0.0.0/16")},
	}

	cidr, err := allocator.FindNextAvailableInParent("10.0.0.0/16", existing, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.16.0/20" {
		t.Errorf("expected 10.0.16.0/20, got %s", cidr)
	}
}

func TestFindNextAvailableInParent_AlignsToBoundary(t *testing.T) {
	allocator := NewAllocator()

	// Allocate a /25, then try to get a /24
	// The /24 should align to the next /24 boundary
	existing := []Allocation{
		{CIDR: "10.0.0.0/25", ID: "test-1", ParentCIDR: strPtr("10.0.0.0/16")},
	}

	cidr, err := allocator.FindNextAvailableInParent("10.0.0.0/16", existing, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.1.0/24" {
		t.Errorf("expected 10.0.1.0/24, got %s", cidr)
	}
}

func TestValidateNoOverlap_NoOverlap(t *testing.T) {
	allocator := NewAllocator()

	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1"},
	}

	err := allocator.ValidateNoOverlap(existing, "10.0.1.0/24")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateNoOverlap_Overlaps(t *testing.T) {
	allocator := NewAllocator()

	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1"},
	}

	// 10.0.0.128/25 is within 10.0.0.0/24
	err := allocator.ValidateNoOverlap(existing, "10.0.0.128/25")
	if err == nil {
		t.Error("expected overlap error")
	}
}

func TestValidateNoOverlap_ContainedBy(t *testing.T) {
	allocator := NewAllocator()

	existing := []Allocation{
		{CIDR: "10.0.0.128/25", ID: "test-1"},
	}

	// 10.0.0.0/24 contains 10.0.0.128/25
	err := allocator.ValidateNoOverlap(existing, "10.0.0.0/24")
	if err == nil {
		t.Error("expected overlap error")
	}
}

func TestCalculateAvailableSpace(t *testing.T) {
	allocator := NewAllocator()

	// Empty /24 should have 256 addresses
	available, err := allocator.CalculateAvailableSpace("10.0.0.0/24", []Allocation{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if available != 256 {
		t.Errorf("expected 256 available, got %d", available)
	}

	// /24 with one /25 allocated should have 128 addresses
	existing := []Allocation{
		{CIDR: "10.0.0.0/25", ID: "test-1"},
	}
	available, err = allocator.CalculateAvailableSpace("10.0.0.0/24", existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if available != 128 {
		t.Errorf("expected 128 available, got %d", available)
	}
}

func TestFilterTopLevelAllocations(t *testing.T) {
	allocations := []Allocation{
		{CIDR: "10.0.0.0/16", ID: "test-1", ParentCIDR: nil},
		{CIDR: "10.0.0.0/24", ID: "test-2", ParentCIDR: strPtr("10.0.0.0/16")},
		{CIDR: "10.1.0.0/16", ID: "test-3", ParentCIDR: nil},
	}

	topLevel := filterTopLevelAllocations(allocations)
	if len(topLevel) != 2 {
		t.Errorf("expected 2 top-level allocations, got %d", len(topLevel))
	}
}

// Helper function to create a string pointer
func strPtr(s string) *string {
	return &s
}

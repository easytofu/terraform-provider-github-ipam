// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"net"
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

// Additional comprehensive tests

func TestFindNextAvailableInPool_MultipleCIDRs(t *testing.T) {
	allocator := NewAllocator()
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/24", "10.0.1.0/24"}, // Two CIDRs
	}

	// Fill the first CIDR
	existing := []Allocation{
		{CIDR: "10.0.0.0/25", ID: "test-1"},
		{CIDR: "10.0.0.128/25", ID: "test-2"},
	}

	// Should allocate from second CIDR
	cidr, err := allocator.FindNextAvailableInPool(poolDef, existing, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.1.0/25" {
		t.Errorf("expected 10.0.1.0/25, got %s", cidr)
	}
}

func TestFindNextAvailableInPool_SkipsInvalidCIDRs(t *testing.T) {
	allocator := NewAllocator()
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/24"},
	}

	// Include an invalid CIDR that should be skipped
	existing := []Allocation{
		{CIDR: "invalid-cidr", ID: "invalid"},
		{CIDR: "10.0.0.0/25", ID: "valid"},
	}

	cidr, err := allocator.FindNextAvailableInPool(poolDef, existing, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.0.128/25" {
		t.Errorf("expected 10.0.0.128/25, got %s", cidr)
	}
}

func TestFindNextAvailableInPool_DifferentMasks(t *testing.T) {
	tests := []struct {
		name         string
		poolCIDR     string
		existingCIDR string
		requestMask  int
		expected     string
	}{
		{"request /16 in /8", "10.0.0.0/8", "", 16, "10.0.0.0/16"},
		{"request /20 in /16", "10.0.0.0/16", "", 20, "10.0.0.0/20"},
		{"request /28 in /24", "10.0.0.0/24", "", 28, "10.0.0.0/28"},
		{"second /16 in /8", "10.0.0.0/8", "10.0.0.0/16", 16, "10.1.0.0/16"},
	}

	allocator := NewAllocator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poolDef := &PoolDefinition{CIDR: []string{tt.poolCIDR}}
			var existing []Allocation
			if tt.existingCIDR != "" {
				existing = []Allocation{{CIDR: tt.existingCIDR, ID: "existing"}}
			}

			cidr, err := allocator.FindNextAvailableInPool(poolDef, existing, tt.requestMask)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cidr != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, cidr)
			}
		})
	}
}

func TestFindNextAvailableInPool_BoundaryConditions(t *testing.T) {
	allocator := NewAllocator()

	// Test at the end of a range
	poolDef := &PoolDefinition{
		CIDR: []string{"10.0.0.0/24"},
	}

	// Fill all but the last /26
	existing := []Allocation{
		{CIDR: "10.0.0.0/26", ID: "test-1"},
		{CIDR: "10.0.0.64/26", ID: "test-2"},
		{CIDR: "10.0.0.128/26", ID: "test-3"},
	}

	cidr, err := allocator.FindNextAvailableInPool(poolDef, existing, 26)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.0.192/26" {
		t.Errorf("expected 10.0.0.192/26 (last /26 in range), got %s", cidr)
	}
}

func TestFindNextAvailableInParent_Exhausted(t *testing.T) {
	allocator := NewAllocator()

	// Parent is /24, fill with two /25s
	existing := []Allocation{
		{CIDR: "10.0.0.0/25", ID: "test-1", ParentCIDR: strPtr("10.0.0.0/24")},
		{CIDR: "10.0.0.128/25", ID: "test-2", ParentCIDR: strPtr("10.0.0.0/24")},
	}

	_, err := allocator.FindNextAvailableInParent("10.0.0.0/24", existing, 25)
	if err == nil {
		t.Error("expected error for exhausted parent")
	}
}

func TestFindNextAvailableInParent_InvalidParentCIDR(t *testing.T) {
	allocator := NewAllocator()

	_, err := allocator.FindNextAvailableInParent("not-a-cidr", []Allocation{}, 24)
	if err == nil {
		t.Error("expected error for invalid parent CIDR")
	}
}

func TestFindNextAvailableInParent_PrefixTooLarge(t *testing.T) {
	allocator := NewAllocator()

	// Try to allocate a /16 from a /24 parent
	_, err := allocator.FindNextAvailableInParent("10.0.0.0/24", []Allocation{}, 16)
	if err == nil {
		t.Error("expected error for prefix larger than parent")
	}
}

func TestFindNextAvailableInParent_FiltersToParentOnly(t *testing.T) {
	allocator := NewAllocator()

	// Allocations from different parents
	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1", ParentCIDR: strPtr("10.0.0.0/16")},
		{CIDR: "10.1.0.0/24", ID: "test-2", ParentCIDR: strPtr("10.1.0.0/16")}, // Different parent
	}

	// Should find 10.0.1.0/24 (next in 10.0.0.0/16)
	cidr, err := allocator.FindNextAvailableInParent("10.0.0.0/16", existing, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "10.0.1.0/24" {
		t.Errorf("expected 10.0.1.0/24, got %s", cidr)
	}
}

func TestValidateNoOverlap_ExactMatch(t *testing.T) {
	allocator := NewAllocator()

	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1", Name: "existing"},
	}

	err := allocator.ValidateNoOverlap(existing, "10.0.0.0/24")
	if err == nil {
		t.Error("expected overlap error for exact match")
	}
	if err != nil && !containsString(err.Error(), "overlaps") {
		t.Errorf("error should mention 'overlaps': %v", err)
	}
}

func TestValidateNoOverlap_PartialOverlap(t *testing.T) {
	allocator := NewAllocator()

	existing := []Allocation{
		{CIDR: "10.0.0.0/16", ID: "test-1"},
	}

	// 10.0.128.0/17 overlaps with 10.0.0.0/16
	err := allocator.ValidateNoOverlap(existing, "10.0.128.0/17")
	if err == nil {
		t.Error("expected overlap error for partial overlap")
	}
}

func TestValidateNoOverlap_InvalidNewCIDR(t *testing.T) {
	allocator := NewAllocator()

	err := allocator.ValidateNoOverlap([]Allocation{}, "invalid")
	if err == nil {
		t.Error("expected error for invalid new CIDR")
	}
}

func TestValidateNoOverlap_SkipsInvalidExisting(t *testing.T) {
	allocator := NewAllocator()

	existing := []Allocation{
		{CIDR: "invalid-cidr", ID: "invalid"},
		{CIDR: "10.1.0.0/24", ID: "valid"},
	}

	// Should not error because 10.0.0.0/24 doesn't overlap with 10.1.0.0/24
	err := allocator.ValidateNoOverlap(existing, "10.0.0.0/24")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCalculateAvailableSpace_FullPool(t *testing.T) {
	allocator := NewAllocator()

	// /24 with all space allocated
	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1"},
	}

	available, err := allocator.CalculateAvailableSpace("10.0.0.0/24", existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if available != 0 {
		t.Errorf("expected 0 available, got %d", available)
	}
}

func TestCalculateAvailableSpace_MixedSizes(t *testing.T) {
	allocator := NewAllocator()

	// /16 with mixed allocations
	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1"},   // 256 addresses
		{CIDR: "10.0.1.0/25", ID: "test-2"},   // 128 addresses
		{CIDR: "10.0.1.128/26", ID: "test-3"}, // 64 addresses
	}

	// Total /16 = 65536, allocated = 256 + 128 + 64 = 448
	available, err := allocator.CalculateAvailableSpace("10.0.0.0/16", existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := uint64(65536 - 448)
	if available != expected {
		t.Errorf("expected %d available, got %d", expected, available)
	}
}

func TestCalculateAvailableSpace_InvalidContainerCIDR(t *testing.T) {
	allocator := NewAllocator()

	_, err := allocator.CalculateAvailableSpace("invalid", []Allocation{})
	if err == nil {
		t.Error("expected error for invalid container CIDR")
	}
}

func TestCalculateAvailableSpace_FiltersToContainer(t *testing.T) {
	allocator := NewAllocator()

	// Allocations from different ranges
	existing := []Allocation{
		{CIDR: "10.0.0.0/24", ID: "test-1"},   // In container
		{CIDR: "10.1.0.0/24", ID: "test-2"},   // Outside container
		{CIDR: "192.168.0.0/24", ID: "test-3"}, // Outside container
	}

	// Only 10.0.0.0/24 should be counted
	available, err := allocator.CalculateAvailableSpace("10.0.0.0/16", existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := uint64(65536 - 256) // Only one /24 counted
	if available != expected {
		t.Errorf("expected %d available, got %d", expected, available)
	}
}

func TestCompareIPs(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected int
	}{
		{"equal", "10.0.0.1", "10.0.0.1", 0},
		{"a < b", "10.0.0.1", "10.0.0.2", -1},
		{"a > b", "10.0.0.2", "10.0.0.1", 1},
		{"different octets", "10.0.0.0", "10.0.1.0", -1},
		{"large difference", "10.0.0.0", "192.168.0.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := parseIP(tt.a)
			b := parseIP(tt.b)
			result := compareIPs(a, b)
			if result != tt.expected {
				t.Errorf("compareIPs(%s, %s) = %d, want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestCloneIP(t *testing.T) {
	original := parseIP("10.0.0.1")
	cloned := cloneIP(original)

	// Should be equal
	if compareIPs(original, cloned) != 0 {
		t.Error("cloned IP should equal original")
	}

	// Should be independent
	cloned[3] = 99
	if original[3] == 99 {
		t.Error("modifying clone should not affect original")
	}
}

func TestAlignToPrefix(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		prefixLen int
		expected  string
	}{
		{"already aligned /24", "10.0.0.0", 24, "10.0.0.0"},
		{"already aligned /16", "10.0.0.0", 16, "10.0.0.0"},
		{"needs alignment /24", "10.0.0.50", 24, "10.0.1.0"},
		{"needs alignment /16", "10.0.0.50", 16, "10.1.0.0"},
		{"edge of boundary", "10.0.0.255", 24, "10.0.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(tt.ip)
			result := alignToPrefix(ip, tt.prefixLen, 32)
			expected := parseIP(tt.expected)
			if compareIPs(result, expected) != 0 {
				t.Errorf("alignToPrefix(%s, %d) = %s, want %s", tt.ip, tt.prefixLen, result.String(), tt.expected)
			}
		})
	}
}

// Helper functions

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStringHelper(s, substr))
}

func containsStringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func parseIP(s string) net.IP {
	return net.ParseIP(s)
}

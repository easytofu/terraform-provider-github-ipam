// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"strings"
	"testing"
)

func TestGenerateAllFiles_EmptyState(t *testing.T) {
	pools := NewPoolsConfig()
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)

	if result == nil {
		t.Fatal("GenerateAllFiles should return non-nil")
		return
	}

	// Should at least have the main README
	if _, exists := result.Files[".github/README.md"]; !exists {
		t.Error("should include .github/README.md")
	}
}

func TestGenerateAllFiles_WithPools(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test-pool", PoolDefinition{
		CIDR:        []string{"10.0.0.0/16"},
		Description: "Test pool",
	})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)

	// Should have main README and pool page
	if _, exists := result.Files[".github/README.md"]; !exists {
		t.Error("should include .github/README.md")
	}
	if _, exists := result.Files[".github/ipam/pools/test-pool.md"]; !exists {
		t.Error("should include pool page")
	}
}

func TestGenerateAllFiles_MultiplePools(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("pool-a", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	pools.AddPool("pool-b", PoolDefinition{CIDR: []string{"10.1.0.0/16"}})
	pools.AddPool("pool-c", PoolDefinition{CIDR: []string{"172.16.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)

	expectedFiles := []string{
		".github/README.md",
		".github/ipam/pools/pool-a.md",
		".github/ipam/pools/pool-b.md",
		".github/ipam/pools/pool-c.md",
	}

	for _, expected := range expectedFiles {
		if _, exists := result.Files[expected]; !exists {
			t.Errorf("missing expected file: %s", expected)
		}
	}
}

func TestMainREADME_ContainsTitle(t *testing.T) {
	pools := NewPoolsConfig()
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "# IP Address Space Overview") {
		t.Error("README should contain title")
	}
}

func TestMainREADME_ContainsWarning(t *testing.T) {
	pools := NewPoolsConfig()
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "**IMPORTANT**") {
		t.Error("README should contain IMPORTANT warning")
	}
	if !strings.Contains(readme, "automatically generated") {
		t.Error("README should mention auto-generation")
	}
}

func TestMainREADME_ContainsLegend(t *testing.T) {
	pools := NewPoolsConfig()
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "Allocated") {
		t.Error("README should contain 'Allocated' in legend")
	}
	if !strings.Contains(readme, "Reserved") {
		t.Error("README should contain 'Reserved' in legend")
	}
	if !strings.Contains(readme, "Available") {
		t.Error("README should contain 'Available' in legend")
	}
}

func TestMainREADME_ContainsClassASections(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "Class A Address Space") {
		t.Error("README should contain Class A section")
	}
	if !strings.Contains(readme, "10.0.0.0 - 10.255.255.255") {
		t.Error("README should contain Class A range")
	}
}

func TestMainREADME_ContainsClassBSections(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{CIDR: []string{"172.16.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "Class B Address Space") {
		t.Error("README should contain Class B section")
	}
	if !strings.Contains(readme, "172.16.0.0 - 172.31.255.255") {
		t.Error("README should contain Class B range")
	}
}

func TestMainREADME_ContainsClassCSections(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{CIDR: []string{"192.168.0.0/18"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "Class C Address Space") {
		t.Error("README should contain Class C section")
	}
	if !strings.Contains(readme, "192.168.0.0 - 192.168.255.255") {
		t.Error("README should contain Class C range")
	}
}

func TestMainREADME_PoolLinkIncluded(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("my-pool", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "[my-pool](ipam/pools/my-pool.md)") {
		t.Error("README should contain link to pool page")
	}
}

func TestMainREADME_ReservedPoolIcon(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("reserved-pool", PoolDefinition{
		CIDR:     []string{"10.0.0.0/16"},
		Reserved: true,
	})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	// Should have reserved icon for this pool
	if !strings.Contains(readme, "Reserved") {
		t.Error("README should show Reserved status for reserved pool")
	}
}

func TestMainREADME_ContainsTotals(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	if !strings.Contains(readme, "**Totals**") {
		t.Error("README should contain Totals row")
	}
}

func TestMainREADME_NoHCL(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test-pool", PoolDefinition{
		CIDR:        []string{"10.0.0.0/16"},
		Description: "Test pool",
	})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	readme := result.Files[".github/README.md"]

	// HCL sections have been removed from documentation
	if strings.Contains(readme, "```hcl") {
		t.Error("README should not contain HCL code blocks")
	}
}

func TestPoolPage_ContainsTitle(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("my-pool", PoolDefinition{
		CIDR:        []string{"10.0.0.0/16"},
		Description: "My test pool",
	})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/my-pool.md"]

	if !strings.Contains(poolPage, "# my-pool") {
		t.Error("pool page should contain pool name as title")
	}
}

func TestPoolPage_ContainsWarning(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/test.md"]

	if !strings.Contains(poolPage, "**IMPORTANT**") {
		t.Error("pool page should contain IMPORTANT warning")
	}
}

func TestPoolPage_ContainsBackLink(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/test.md"]

	if !strings.Contains(poolPage, "[← Back to Overview](../README.md)") {
		t.Error("pool page should contain back link")
	}
}

func TestPoolPage_ContainsDescription(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{
		CIDR:        []string{"10.0.0.0/16"},
		Description: "My custom description",
	})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/test.md"]

	if !strings.Contains(poolPage, "My custom description") {
		t.Error("pool page should contain description")
	}
}

func TestPoolPage_ContainsOverviewTable(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/test.md"]

	if !strings.Contains(poolPage, "## Overview") {
		t.Error("pool page should contain Overview section")
	}
	if !strings.Contains(poolPage, "Pool Name") {
		t.Error("pool page should contain Pool Name in overview")
	}
	if !strings.Contains(poolPage, "CIDR") {
		t.Error("pool page should contain CIDR in overview")
	}
}

func TestPoolPage_ContainsMetadata(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("test", PoolDefinition{
		CIDR: []string{"10.0.0.0/16"},
		Metadata: map[string]string{
			"environment": "production",
			"product":     "platform",
		},
	})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/test.md"]

	if !strings.Contains(poolPage, "Environment") {
		t.Error("pool page should contain Environment metadata (title case)")
	}
	if !strings.Contains(poolPage, "production") {
		t.Error("pool page should contain metadata value")
	}
}

func TestPoolPage_ReservedBanner(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("reserved", PoolDefinition{
		CIDR:     []string{"10.0.0.0/16"},
		Reserved: true,
	})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/reserved.md"]

	if !strings.Contains(poolPage, "RESERVED") {
		t.Error("reserved pool page should contain RESERVED banner")
	}
}

func TestPoolPage_EmptyPoolShowsAvailable(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("empty", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/empty.md"]

	// Empty pools should show available space
	if !strings.Contains(poolPage, "Available") {
		t.Error("empty pool page should show 'Available' row")
	}
}

func TestPoolPage_WithAllocations(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("prod", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()
	allocs.AddAllocation("prod", Allocation{
		CIDR: "10.0.0.0/24",
		ID:   "id-1",
		Name: "vpc-main",
	})

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/prod.md"]

	if !strings.Contains(poolPage, "## Allocations") {
		t.Error("pool page should contain Allocations section")
	}
	if !strings.Contains(poolPage, "vpc-main") {
		t.Error("pool page should contain allocation name")
	}
	if !strings.Contains(poolPage, "10.0.0.0/24") {
		t.Error("pool page should contain allocation CIDR")
	}
}

func TestPoolPage_ReservedAllocationIcon(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("prod", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()
	allocs.AddAllocation("prod", Allocation{
		CIDR:     "10.0.0.0/24",
		ID:       "id-1",
		Name:     "reserved-block",
		Reserved: true,
	})

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/prod.md"]

	if !strings.Contains(poolPage, "Reserved") {
		t.Error("pool page should show Reserved status for reserved allocation")
	}
}

func TestPoolPage_NoAllocationHCL(t *testing.T) {
	pools := NewPoolsConfig()
	pools.AddPool("prod", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	allocs := NewAllocationsDatabase()
	allocs.AddAllocation("prod", Allocation{
		CIDR: "10.0.0.0/24",
		ID:   "id-1",
		Name: "vpc-main",
	})

	result := GenerateAllFiles(pools, allocs)
	poolPage := result.Files[".github/ipam/pools/prod.md"]

	// HCL sections have been removed from documentation
	if strings.Contains(poolPage, "```hcl") {
		t.Error("pool page should not contain HCL code blocks")
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{100, "100"},
		{999, "999"},
		{1000, "1,000"},
		{10000, "10,000"},
		{100000, "100,000"},
		{1000000, "1,000,000"},
		{1048576, "1,048,576"},
		{16777216, "16,777,216"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatNumber(tt.input)
			if result != tt.expected {
				t.Errorf("formatNumber(%d) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"environment", "Environment"},
		{"env", "Env"},
		{"PRODUCT", "Product"},
		{"product-name", "Product Name"},
		{"some_key_name", "Some Key Name"},
		{"already Title Case", "Already Title Case"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toTitleCase(tt.input)
			if result != tt.expected {
				t.Errorf("toTitleCase(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRenderUtilBarOnly(t *testing.T) {
	tests := []struct {
		name     string
		pct      float64
		expected string
	}{
		{"0%", 0, "░░░░░░░░░░"},
		{"10%", 10, "█░░░░░░░░░"},
		{"50%", 50, "█████░░░░░"},
		{"100%", 100, "██████████"},
		{"25%", 25, "██░░░░░░░░"},
		{"75%", 75, "███████░░░"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderUtilBarOnly(tt.pct)
			if result != tt.expected {
				t.Errorf("renderUtilBarOnly(%v) = %s, want %s", tt.pct, result, tt.expected)
			}
		})
	}
}

func TestCIDRToAddresses(t *testing.T) {
	tests := []struct {
		cidr     string
		expected uint64
	}{
		{"10.0.0.0/8", 16777216},
		{"10.0.0.0/16", 65536},
		{"10.0.0.0/24", 256},
		{"10.0.0.0/32", 1},
		{"192.168.0.0/16", 65536},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			result := cidrToAddresses(tt.cidr)
			if result != tt.expected {
				t.Errorf("cidrToAddresses(%s) = %d, want %d", tt.cidr, result, tt.expected)
			}
		})
	}
}

func TestCIDRToAddresses_Invalid(t *testing.T) {
	result := cidrToAddresses("not-a-cidr")
	if result != 0 {
		t.Errorf("cidrToAddresses with invalid input should return 0, got %d", result)
	}
}

func TestUint32ToIP(t *testing.T) {
	tests := []struct {
		input    uint32
		expected string
	}{
		{0, "0.0.0.0"},
		{167772160, "10.0.0.0"},    // 10.0.0.0
		{167772416, "10.0.1.0"},    // 10.0.1.0
		{4294967295, "255.255.255.255"},
		{3232235520, "192.168.0.0"}, // 192.168.0.0
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := uint32ToIP(tt.input)
			if result != tt.expected {
				t.Errorf("uint32ToIP(%d) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCompareCIDRs(t *testing.T) {
	tests := []struct {
		name     string
		a        string
		b        string
		expected bool
	}{
		{"same CIDR", "10.0.0.0/24", "10.0.0.0/24", false},
		{"a before b", "10.0.0.0/24", "10.0.1.0/24", true},
		{"a after b", "10.0.1.0/24", "10.0.0.0/24", false},
		{"different classes", "10.0.0.0/8", "192.168.0.0/16", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareCIDRs(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("compareCIDRs(%s, %s) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

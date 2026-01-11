// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"net"
	"testing"
)

func TestNewPoolsConfig(t *testing.T) {
	config := NewPoolsConfig()
	if config == nil {
		t.Fatal("NewPoolsConfig returned nil")
		return
	}
	if config.Pools == nil {
		t.Error("Pools map should be initialized")
		return
	}
	if len(config.Pools) != 0 {
		t.Errorf("Pools map should be empty, got %d entries", len(config.Pools))
	}
}

func TestPoolsConfig_GetPool_Exists(t *testing.T) {
	config := NewPoolsConfig()
	config.Pools["test-pool"] = PoolDefinition{
		CIDR:        []string{"10.0.0.0/16"},
		Description: "Test pool",
	}

	pool, exists := config.GetPool("test-pool")
	if !exists {
		t.Error("expected pool to exist")
		return
	}
	if pool == nil {
		t.Fatal("pool should not be nil")
		return
	}
	if pool.Description != "Test pool" {
		t.Errorf("expected description 'Test pool', got '%s'", pool.Description)
	}
}

func TestPoolsConfig_GetPool_NotExists(t *testing.T) {
	config := NewPoolsConfig()

	pool, exists := config.GetPool("missing")
	if exists {
		t.Error("expected pool to not exist")
	}
	if pool != nil {
		t.Error("pool should be nil when not found")
	}
}

func TestPoolsConfig_GetPool_NilPools(t *testing.T) {
	config := &PoolsConfig{Pools: nil}

	pool, exists := config.GetPool("any")
	if exists {
		t.Error("expected pool to not exist with nil Pools")
	}
	if pool != nil {
		t.Error("pool should be nil")
	}
}

func TestPoolsConfig_ListPoolIDs_Empty(t *testing.T) {
	config := NewPoolsConfig()

	ids := config.ListPoolIDs()
	if ids == nil {
		t.Error("ListPoolIDs should return empty slice, not nil")
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 pool IDs, got %d", len(ids))
	}
}

func TestPoolsConfig_ListPoolIDs_Multiple(t *testing.T) {
	config := NewPoolsConfig()
	config.Pools["pool-a"] = PoolDefinition{CIDR: []string{"10.0.0.0/16"}}
	config.Pools["pool-b"] = PoolDefinition{CIDR: []string{"10.1.0.0/16"}}
	config.Pools["pool-c"] = PoolDefinition{CIDR: []string{"10.2.0.0/16"}}

	ids := config.ListPoolIDs()
	if len(ids) != 3 {
		t.Errorf("expected 3 pool IDs, got %d", len(ids))
	}

	// Check all IDs are present (order not guaranteed)
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}
	for _, expected := range []string{"pool-a", "pool-b", "pool-c"} {
		if !idMap[expected] {
			t.Errorf("expected pool ID '%s' not found", expected)
		}
	}
}

func TestPoolsConfig_ListPoolIDs_NilPools(t *testing.T) {
	config := &PoolsConfig{Pools: nil}

	ids := config.ListPoolIDs()
	if ids != nil {
		t.Error("ListPoolIDs should return nil when Pools is nil")
	}
}

func TestPoolsConfig_AddPool_NewPool(t *testing.T) {
	config := NewPoolsConfig()

	pool := PoolDefinition{
		CIDR:        []string{"10.0.0.0/16"},
		Description: "New pool",
		Metadata:    map[string]string{"env": "prod"},
		Reserved:    false,
	}

	config.AddPool("new-pool", pool)

	retrieved, exists := config.GetPool("new-pool")
	if !exists {
		t.Fatal("pool should exist after adding")
	}
	if retrieved.Description != "New pool" {
		t.Errorf("description mismatch: got '%s'", retrieved.Description)
	}
	if len(retrieved.CIDR) != 1 || retrieved.CIDR[0] != "10.0.0.0/16" {
		t.Error("CIDR mismatch")
	}
}

func TestPoolsConfig_AddPool_UpdateExisting(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("test", PoolDefinition{Description: "Original"})

	config.AddPool("test", PoolDefinition{Description: "Updated"})

	retrieved, _ := config.GetPool("test")
	if retrieved.Description != "Updated" {
		t.Errorf("expected 'Updated', got '%s'", retrieved.Description)
	}
}

func TestPoolsConfig_AddPool_NilPools(t *testing.T) {
	config := &PoolsConfig{Pools: nil}

	config.AddPool("test", PoolDefinition{Description: "Test"})

	if config.Pools == nil {
		t.Error("Pools should be initialized by AddPool")
	}
	retrieved, exists := config.GetPool("test")
	if !exists || retrieved.Description != "Test" {
		t.Error("pool should be added correctly")
	}
}

func TestPoolsConfig_RemovePool_Exists(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("test", PoolDefinition{Description: "Test"})

	err := config.RemovePool("test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	_, exists := config.GetPool("test")
	if exists {
		t.Error("pool should not exist after removal")
	}
}

func TestPoolsConfig_RemovePool_NotExists(t *testing.T) {
	config := NewPoolsConfig()

	err := config.RemovePool("missing")
	if err == nil {
		t.Error("expected error when removing non-existent pool")
	}
}

func TestPoolsConfig_RemovePool_NilPools(t *testing.T) {
	config := &PoolsConfig{Pools: nil}

	err := config.RemovePool("any")
	if err == nil {
		t.Error("expected error when Pools is nil")
	}
}

func TestPoolsConfig_ValidatePools_Empty(t *testing.T) {
	config := NewPoolsConfig()

	err := config.ValidatePools()
	if err != nil {
		t.Errorf("unexpected error for empty pools: %v", err)
	}
}

func TestPoolsConfig_ValidatePools_NilPools(t *testing.T) {
	config := &PoolsConfig{Pools: nil}

	err := config.ValidatePools()
	if err != nil {
		t.Errorf("unexpected error for nil pools: %v", err)
	}
}

func TestPoolsConfig_ValidatePools_ValidSingle(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("test", PoolDefinition{
		CIDR: []string{"10.0.0.0/16"},
	})

	err := config.ValidatePools()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPoolsConfig_ValidatePools_ValidMultiple(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("pool-a", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	config.AddPool("pool-b", PoolDefinition{CIDR: []string{"10.1.0.0/16"}})
	config.AddPool("pool-c", PoolDefinition{CIDR: []string{"172.16.0.0/16"}})

	err := config.ValidatePools()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPoolsConfig_ValidatePools_NoCIDRs(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("empty", PoolDefinition{CIDR: []string{}})

	err := config.ValidatePools()
	if err == nil {
		t.Error("expected error for pool with no CIDRs")
	}
}

func TestPoolsConfig_ValidatePools_InvalidCIDR(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("invalid", PoolDefinition{CIDR: []string{"not-a-cidr"}})

	err := config.ValidatePools()
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestPoolsConfig_ValidatePools_OverlappingPools(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("pool-a", PoolDefinition{CIDR: []string{"10.0.0.0/8"}})
	config.AddPool("pool-b", PoolDefinition{CIDR: []string{"10.0.0.0/16"}}) // Overlaps with pool-a

	err := config.ValidatePools()
	if err == nil {
		t.Error("expected error for overlapping pools")
	}
}

func TestPoolsConfig_ValidatePools_AdjacentNonOverlapping(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("pool-a", PoolDefinition{CIDR: []string{"10.0.0.0/16"}})
	config.AddPool("pool-b", PoolDefinition{CIDR: []string{"10.1.0.0/16"}})

	err := config.ValidatePools()
	if err != nil {
		t.Errorf("adjacent pools should not overlap: %v", err)
	}
}

func TestPoolsConfig_ValidatePools_MultipleCIDRsInPool(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("multi", PoolDefinition{
		CIDR: []string{"10.0.0.0/16", "10.1.0.0/16"},
	})

	err := config.ValidatePools()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPoolsConfig_ValidatePools_MultipleCIDRsOverlap(t *testing.T) {
	config := NewPoolsConfig()
	config.AddPool("multi", PoolDefinition{
		CIDR: []string{"10.0.0.0/8", "10.0.0.0/16"}, // Second overlaps first
	})

	err := config.ValidatePools()
	if err == nil {
		t.Error("expected error for overlapping CIDRs within same pool")
	}
}

func TestNetworksOverlap_Disjoint(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"disjoint /16s", "10.0.0.0/16", "10.1.0.0/16", false},
		{"disjoint classes", "10.0.0.0/8", "172.16.0.0/12", false},
		{"adjacent /24s", "10.0.0.0/24", "10.0.1.0/24", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mustParseCIDR(t, tt.a)
			b := mustParseCIDR(t, tt.b)
			if got := networksOverlap(a, b); got != tt.want {
				t.Errorf("networksOverlap(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNetworksOverlap_Overlapping(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"exact same", "10.0.0.0/16", "10.0.0.0/16", true},
		{"contained", "10.0.0.0/8", "10.0.0.0/16", true},
		{"contains", "10.0.0.0/16", "10.0.0.0/8", true},
		{"partial overlap", "10.0.0.0/15", "10.1.0.0/16", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mustParseCIDR(t, tt.a)
			b := mustParseCIDR(t, tt.b)
			if got := networksOverlap(a, b); got != tt.want {
				t.Errorf("networksOverlap(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestPoolDefinition_Reserved(t *testing.T) {
	pool := PoolDefinition{
		CIDR:     []string{"10.0.0.0/16"},
		Reserved: true,
	}

	if !pool.Reserved {
		t.Error("pool should be reserved")
	}
}

func TestPoolDefinition_Metadata(t *testing.T) {
	pool := PoolDefinition{
		CIDR: []string{"10.0.0.0/16"},
		Metadata: map[string]string{
			"env":     "production",
			"product": "platform",
		},
	}

	if pool.Metadata["env"] != "production" {
		t.Error("metadata env should be 'production'")
	}
	if pool.Metadata["product"] != "platform" {
		t.Error("metadata product should be 'platform'")
	}
}

// Helper function
func mustParseCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("failed to parse CIDR %s: %v", cidr, err)
	}
	return network
}

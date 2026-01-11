// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"fmt"
	"math/big"
	"net"
	"sort"

	"github.com/apparentlymart/go-cidr/cidr"
)

// Allocator provides CIDR allocation functionality.
type Allocator struct{}

// NewAllocator creates a new CIDR allocator.
func NewAllocator() *Allocator {
	return &Allocator{}
}

// sortableAllocation is used for sorting allocations by network address.
type sortableAllocation struct {
	network *net.IPNet
	alloc   *Allocation
}

// FindNextAvailableInPool allocates from pool CIDRs defined in pools.yaml.
// This is Mode 1: pool_id allocation.
func (a *Allocator) FindNextAvailableInPool(poolDef *PoolDefinition, existingAllocations []Allocation, prefixLen int) (string, error) {
	// Get top-level allocations (those without parent_cidr)
	topLevelAllocations := filterTopLevelAllocations(existingAllocations)

	// Track reasons for skipping each CIDR
	var skippedReasons []string

	// Try each CIDR in the pool until we find available space
	for _, poolCIDRStr := range poolDef.CIDR {
		cidrResult, err := a.findNextInCIDR(poolCIDRStr, topLevelAllocations, prefixLen)
		if err == nil {
			return cidrResult, nil
		}
		// Record why this CIDR was skipped
		skippedReasons = append(skippedReasons, fmt.Sprintf("%s: %v", poolCIDRStr, err))
	}

	if len(skippedReasons) == 1 {
		return "", fmt.Errorf("no available /%d block in pool: %s", prefixLen, skippedReasons[0])
	}
	return "", fmt.Errorf("no available /%d block in pool (tried %d CIDRs): %v", prefixLen, len(poolDef.CIDR), skippedReasons)
}

// FindNextAvailableInParent allocates within an existing allocation's CIDR.
// This is Mode 2: parent_cidr sub-allocation.
func (a *Allocator) FindNextAvailableInParent(parentCIDR string, childAllocations []Allocation, prefixLen int) (string, error) {
	return a.findNextInCIDR(parentCIDR, childAllocations, prefixLen)
}

// findNextInCIDR finds the next available CIDR block within a given container CIDR.
func (a *Allocator) findNextInCIDR(containerCIDR string, existingAllocations []Allocation, prefixLen int) (string, error) {
	_, containerNet, err := net.ParseCIDR(containerCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid container CIDR %s: %w", containerCIDR, err)
	}

	containerPrefixLen, bits := containerNet.Mask.Size()
	if prefixLen < containerPrefixLen {
		return "", fmt.Errorf("requested prefix /%d is larger than container /%d", prefixLen, containerPrefixLen)
	}
	if prefixLen > bits {
		return "", fmt.Errorf("requested prefix /%d exceeds address size /%d", prefixLen, bits)
	}

	// Filter allocations that are within this container
	relevantAllocations := filterAllocationsInCIDR(existingAllocations, containerNet)

	// Parse and sort existing allocations by network address
	sortable := make([]sortableAllocation, 0, len(relevantAllocations))
	var skippedInvalid int
	for i := range relevantAllocations {
		_, network, err := net.ParseCIDR(relevantAllocations[i].CIDR)
		if err != nil {
			skippedInvalid++
			continue // Skip invalid entries
		}
		sortable = append(sortable, sortableAllocation{
			network: network,
			alloc:   &relevantAllocations[i],
		})
	}
	// Note: skippedInvalid count is available for debugging but not exposed in errors
	// as invalid allocations in the database should be fixed separately
	_ = skippedInvalid

	// Sort by network address
	sort.Slice(sortable, func(i, j int) bool {
		return compareIPs(sortable[i].network.IP, sortable[j].network.IP) < 0
	})

	// Start at the beginning of the container
	candidateIP := cloneIP(containerNet.IP)

	// Align candidate to the requested prefix length boundary
	candidateIP = alignToPrefix(candidateIP, prefixLen, bits)

	for _, existing := range sortable {
		// Create candidate network at current position
		candidateNet := &net.IPNet{
			IP:   candidateIP,
			Mask: net.CIDRMask(prefixLen, bits),
		}

		// Get the end of the candidate block
		_, candidateEnd := cidr.AddressRange(candidateNet)

		// Check if candidate fits before existing allocation
		if compareIPs(candidateEnd, existing.network.IP) <= 0 {
			// Candidate fits in gap before existing
			// Verify it's within the container
			if containerNet.Contains(candidateIP) && containerNet.Contains(candidateEnd) {
				return candidateNet.String(), nil
			}
		}

		// Move candidate past the existing allocation
		_, existingEnd := cidr.AddressRange(existing.network)
		candidateIP = cidr.Inc(existingEnd)

		// Align to prefix boundary
		candidateIP = alignToPrefix(candidateIP, prefixLen, bits)
	}

	// Check if there's space after the last allocation
	candidateNet := &net.IPNet{
		IP:   candidateIP,
		Mask: net.CIDRMask(prefixLen, bits),
	}
	_, candidateEnd := cidr.AddressRange(candidateNet)

	// Verify the candidate is within the container
	_, containerEnd := cidr.AddressRange(containerNet)
	if containerNet.Contains(candidateIP) && compareIPs(candidateEnd, containerEnd) <= 0 {
		return candidateNet.String(), nil
	}

	return "", fmt.Errorf("no available /%d block in %s", prefixLen, containerCIDR)
}

// filterTopLevelAllocations returns allocations that have no parent_cidr.
func filterTopLevelAllocations(allocations []Allocation) []Allocation {
	result := make([]Allocation, 0)
	for _, alloc := range allocations {
		if alloc.ParentCIDR == nil {
			result = append(result, alloc)
		}
	}
	return result
}

// filterAllocationsInCIDR returns allocations whose CIDR is within the container.
func filterAllocationsInCIDR(allocations []Allocation, container *net.IPNet) []Allocation {
	result := make([]Allocation, 0)
	for _, alloc := range allocations {
		_, allocNet, err := net.ParseCIDR(alloc.CIDR)
		if err != nil {
			continue
		}
		if container.Contains(allocNet.IP) {
			result = append(result, alloc)
		}
	}
	return result
}

// compareIPs compares two IP addresses.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func compareIPs(a, b net.IP) int {
	a = a.To16()
	b = b.To16()
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// cloneIP creates a copy of an IP address.
func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

// alignToPrefix aligns an IP address to a prefix boundary.
// For example, 10.0.0.5 aligned to /24 becomes 10.0.0.0, but 10.0.1.0 stays 10.0.1.0.
func alignToPrefix(ip net.IP, prefixLen, bits int) net.IP {
	// Convert IP to big.Int
	ipInt := big.NewInt(0)
	ipInt.SetBytes(ip.To16())

	// Calculate the block size for this prefix
	blockSize := big.NewInt(1)
	blockSize.Lsh(blockSize, uint(bits-prefixLen))

	// Calculate how far into the current block we are
	remainder := big.NewInt(0)
	remainder.Mod(ipInt, blockSize)

	// If we're not at a boundary, move to the next boundary
	if remainder.Sign() != 0 {
		ipInt.Sub(ipInt, remainder)
		ipInt.Add(ipInt, blockSize)
	}

	// Convert back to IP
	ipBytes := ipInt.Bytes()
	result := make(net.IP, 16)
	copy(result[16-len(ipBytes):], ipBytes)

	// Return in the same format as input
	if len(ip) == 4 {
		return result.To4()
	}
	return result
}

// ValidateNoOverlap checks if a CIDR overlaps with existing allocations.
func (a *Allocator) ValidateNoOverlap(existingAllocations []Allocation, newCIDR string) error {
	_, newNet, err := net.ParseCIDR(newCIDR)
	if err != nil {
		return fmt.Errorf("invalid CIDR %s: %w", newCIDR, err)
	}

	for _, existing := range existingAllocations {
		_, existingNet, err := net.ParseCIDR(existing.CIDR)
		if err != nil {
			continue
		}

		if networksOverlap(newNet, existingNet) {
			return fmt.Errorf("CIDR %s overlaps with existing allocation %s (%s)",
				newCIDR, existing.CIDR, existing.Name)
		}
	}

	return nil
}

// CalculateAvailableSpace calculates available space in a pool or parent CIDR.
func (a *Allocator) CalculateAvailableSpace(containerCIDR string, allocations []Allocation) (uint64, error) {
	_, containerNet, err := net.ParseCIDR(containerCIDR)
	if err != nil {
		return 0, fmt.Errorf("invalid CIDR: %w", err)
	}

	prefixLen, bits := containerNet.Mask.Size()
	totalAddresses := uint64(1) << uint(bits-prefixLen)

	// Sum allocated addresses
	var allocatedAddresses uint64
	relevant := filterAllocationsInCIDR(allocations, containerNet)
	for _, alloc := range relevant {
		_, allocNet, err := net.ParseCIDR(alloc.CIDR)
		if err != nil {
			continue
		}
		allocPrefixLen, allocBits := allocNet.Mask.Size()
		allocatedAddresses += uint64(1) << uint(allocBits-allocPrefixLen)
	}

	if allocatedAddresses > totalAddresses {
		return 0, nil
	}
	return totalAddresses - allocatedAddresses, nil
}

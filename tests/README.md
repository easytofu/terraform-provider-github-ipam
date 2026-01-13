# Terraform Provider GitHub IPAM - Comprehensive Test Plan

## Overview

This document outlines a comprehensive test suite for the `terraform-provider-github-ipam` provider. The tests cover all resources, data sources, and internal components with both happy and unhappy paths, edge cases, concurrency scenarios, and error conditions.

## Table of Contents

1. [Test Infrastructure](#test-infrastructure)
2. [Provider Configuration Tests](#1-provider-configuration-tests)
3. [Pool Resource Tests](#2-pool-resource-tests)
4. [Allocation Resource Tests](#3-allocation-resource-tests)
5. [Data Source Tests](#4-data-source-tests)
6. [Client/API Tests](#5-clientapi-tests)
7. [IPAM Allocator Tests](#6-ipam-allocator-tests)
8. [README Generator Tests](#7-readme-generator-tests)
9. [Integration Tests](#8-integration-tests)
10. [Performance Tests](#9-performance-tests)

---

## Test Infrastructure

### Mock GitHub Client

All unit tests should use a mock GitHub client that simulates:
- File operations (read/write with SHA tracking)
- Conflict responses (409)
- Rate limiting (429)
- Network timeouts
- Authentication errors (401, 403)
- Not found errors (404)

### Test Fixtures

Standard test fixtures should include:
- Empty pools.yaml
- Pools with various configurations
- Allocations database with hierarchical allocations
- Reserved pools and allocations

---

## 1. Provider Configuration Tests

### 1.1 Required Attributes

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| PROV-001 | Valid minimal configuration | Provider configures with only required attributes | `token`, `owner`, `repository` | Provider initializes successfully |
| PROV-002 | Missing token | Provider fails when token is missing | `owner`, `repository` only | Error: "token is required" |
| PROV-003 | Missing owner | Provider fails when owner is missing | `token`, `repository` only | Error: "owner is required" |
| PROV-004 | Missing repository | Provider fails when repository is missing | `token`, `owner` only | Error: "repository is required" |
| PROV-005 | Empty token | Provider fails with empty string token | `token = ""` | Error: token validation failure |
| PROV-006 | Token from environment | Provider reads token from GITHUB_TOKEN env var | Set `GITHUB_TOKEN` env var | Provider uses env var token |

### 1.2 Optional Attributes with Defaults

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| PROV-010 | Default branch | Branch defaults to "main" | No `branch` specified | `branch = "main"` |
| PROV-011 | Custom branch | Provider uses custom branch | `branch = "develop"` | Provider uses "develop" |
| PROV-012 | Default pools_file | pools_file defaults to correct path | No `pools_file` specified | `pools_file = "network/pools.yaml"` |
| PROV-013 | Custom pools_file | Provider uses custom pools file path | `pools_file = "custom/pools.yaml"` | Provider uses custom path |
| PROV-014 | Default allocations_file | allocations_file defaults correctly | No `allocations_file` specified | `allocations_file = "network/allocations.yaml"` |
| PROV-015 | Custom allocations_file | Provider uses custom allocations path | `allocations_file = "data/allocs.json"` | Provider uses custom path |
| PROV-016 | Default max_retries | max_retries defaults to 10 | No `max_retries` specified | Client has max_retries = 10 |
| PROV-017 | Custom max_retries | Provider uses custom retry count | `max_retries = 5` | Client has max_retries = 5 |
| PROV-018 | Default base_delay_ms | base_delay_ms defaults to 200 | No `base_delay_ms` specified | Client has base_delay = 200ms |
| PROV-019 | Custom base_delay_ms | Provider uses custom delay | `base_delay_ms = 500` | Client has base_delay = 500ms |

### 1.3 Validation Edge Cases

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| PROV-020 | Zero max_retries | Zero retries disables retry logic | `max_retries = 0` | Client has max_retries = 0 |
| PROV-021 | Negative max_retries | Negative value should be rejected | `max_retries = -1` | Error: validation failure |
| PROV-022 | Zero base_delay_ms | Zero delay is accepted | `base_delay_ms = 0` | Client has base_delay = 0ms |
| PROV-023 | Very large max_retries | Large retry count is accepted | `max_retries = 1000` | Client has max_retries = 1000 |

---

## 2. Pool Resource Tests

### 2.1 Create - Happy Path

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| POOL-C001 | Create first pool in Class A | Create pool in empty 10.0.0.0/8 range | `private_range = "10.0.0.0/8"`, `block_size = 12` | `cidr = "10.0.0.0/12"` |
| POOL-C002 | Create first pool in Class B | Create pool in empty 172.16.0.0/12 range | `private_range = "172.16.0.0/12"`, `block_size = 14` | `cidr = "172.16.0.0/14"` |
| POOL-C003 | Create first pool in Class C | Create pool in empty 192.168.0.0/16 range | `private_range = "192.168.0.0/16"`, `block_size = 18` | `cidr = "192.168.0.0/18"` |
| POOL-C004 | Create second pool - finds next block | Create pool when first block taken | Existing pool at 10.0.0.0/12, new `block_size = 12` | `cidr = "10.16.0.0/12"` |
| POOL-C005 | Create pool with description | Pool includes description | `description = "Test pool"` | Pool has description set |
| POOL-C006 | Create pool with metadata | Pool includes metadata map | `metadata = { env = "prod" }` | Pool has metadata set |
| POOL-C007 | Create reserved pool | Pool is marked as reserved | `reserved = true` | Pool has reserved = true |
| POOL-C008 | Create pool - smallest block | Create /28 pool (smallest allowed) | `block_size = 28` | Valid CIDR with /28 prefix |
| POOL-C009 | Create pool - largest block | Create /8 pool (largest allowed) | `block_size = 8` | Valid CIDR with /8 prefix |
| POOL-C010 | Create pool - fills gap | Create pool that fits between existing | Existing at 10.0.0.0/12 and 10.32.0.0/12 | `cidr = "10.16.0.0/12"` |

### 2.2 Create - Unhappy Path

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| POOL-C020 | Duplicate pool name | Pool with same name exists | Existing pool "test", create "test" | Error: "pool with name 'test' already exists" |
| POOL-C021 | Invalid private_range | Range not in allowed list | `private_range = "8.8.8.0/24"` | Validation error: not in OneOf |
| POOL-C022 | Block size too small | block_size < 8 | `block_size = 7` | Validation error: outside range 8-28 |
| POOL-C023 | Block size too large | block_size > 28 | `block_size = 29` | Validation error: outside range 8-28 |
| POOL-C024 | Block larger than range | Request /8 from Class C | `private_range = "192.168.0.0/16"`, `block_size = 8` | Error: prefix larger than parent |
| POOL-C025 | No available space | All space in range allocated | All of 192.168.0.0/16 allocated | Error: "no available block" |
| POOL-C026 | Missing name | No name attribute | Omit `name` | Validation error: name required |
| POOL-C027 | Missing private_range | No private_range attribute | Omit `private_range` | Validation error: private_range required |
| POOL-C028 | Missing block_size | No block_size attribute | Omit `block_size` | Validation error: block_size required |
| POOL-C029 | Empty name | Name is empty string | `name = ""` | Validation error or API error |

### 2.3 Read

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| POOL-R001 | Read existing pool | Pool exists in pools.yaml | Valid pool name | All attributes populated |
| POOL-R002 | Read pool with all attributes | Pool has all optional fields | Pool with description, metadata, reserved | All fields returned correctly |
| POOL-R003 | Read deleted pool | Pool no longer exists | Pool removed from pools.yaml | Resource removed from state |
| POOL-R004 | Read pool - empty metadata | Pool has no metadata | Pool without metadata | `metadata = null` or empty map |

### 2.4 Update

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| POOL-U001 | Update description | Change pool description | `description = "New desc"` | Description updated, CIDR unchanged |
| POOL-U002 | Update metadata | Change pool metadata | `metadata = { new = "value" }` | Metadata updated |
| POOL-U003 | Update reserved flag | Change reserved status | `reserved = true` → `false` | Reserved flag updated |
| POOL-U004 | Add metadata to pool | Pool had no metadata | Add `metadata = { ... }` | Metadata added |
| POOL-U005 | Remove metadata | Pool had metadata | Set `metadata = {}` | Metadata cleared |
| POOL-U006 | Update multiple attributes | Change multiple fields | Update description and metadata | Both updated atomically |

### 2.5 Update - Forces Replacement

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| POOL-U010 | Change name | Attempt to change pool name | `name = "new-name"` | Plan shows destroy/create |
| POOL-U011 | Change private_range | Attempt to change range | `private_range = "172.16.0.0/12"` | Plan shows destroy/create |
| POOL-U012 | Change block_size | Attempt to change block size | `block_size = 16` | Plan shows destroy/create |

### 2.6 Delete

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| POOL-D001 | Delete pool - no allocations | Pool has no allocations | Delete pool | Pool removed from pools.yaml |
| POOL-D002 | Delete pool - already deleted | Pool doesn't exist | Delete pool not in state | Success (idempotent) |
| POOL-D003 | Delete pool - has allocations | Pool has active allocations | Delete pool | Error: "has X active allocations" |
| POOL-D004 | Delete pool - has reservations | Pool has only reservations | Delete pool | Error: "has X active allocations" |
| POOL-D005 | Delete pool - race condition | Allocation added during delete | Delete, allocation added concurrently | Error detected, delete fails |

### 2.7 Import

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| POOL-I001 | Import existing pool | Pool exists in pools.yaml | `terraform import pool.test pool-name` | State populated correctly |
| POOL-I002 | Import non-existent pool | Pool not in pools.yaml | `terraform import pool.test missing` | Error: pool not found |
| POOL-I003 | Import pool with metadata | Pool has metadata | Import pool with metadata | Metadata in state |

---

## 3. Allocation Resource Tests

### 3.1 Create - Mode 1 (pool_id) Happy Path

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-C001 | First allocation in pool | Allocate from empty pool | `pool_id`, `cidr_mask = 16` | First available CIDR in pool |
| ALLOC-C002 | Second allocation | Allocate when one exists | Second allocation | Next available CIDR |
| ALLOC-C003 | Allocation with metadata | Include metadata | `metadata = { ... }` | Allocation has metadata |
| ALLOC-C004 | Create reservation | Status is reservation | `status = "reservation"` | Allocation marked as reserved |
| ALLOC-C005 | Allocation fills gap | Allocate into gap between existing | Gap between allocations | CIDR in gap |
| ALLOC-C006 | Different mask sizes | Various prefix lengths | `cidr_mask = 24`, `20`, `28` | Correct CIDR for each size |
| ALLOC-C007 | Contiguous allocation - before | Allocate adjacent to existing | `contiguous_with = "10.1.0.0/16"` | CIDR at 10.0.0.0/16 |
| ALLOC-C008 | Contiguous allocation - after | Allocate after existing | `contiguous_with = "10.0.0.0/16"` | CIDR at 10.1.0.0/16 |
| ALLOC-C009 | UUID idempotency | Same config gets same UUID | Create with same pool_id/name/mask | Same deterministic UUID |

### 3.2 Create - Mode 2 (parent_cidr) Happy Path

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-C020 | First sub-allocation | Sub-allocate from parent | `parent_cidr`, `cidr_mask = 24` | First /24 in parent |
| ALLOC-C021 | Multiple sub-allocations | Several children in parent | Three /24 allocations | Sequential CIDRs |
| ALLOC-C022 | Nested sub-allocation | Child of a child | Sub-allocate from sub-allocation | Valid nested CIDR |
| ALLOC-C023 | Sub-allocation with metadata | Include metadata | `metadata = { subnet = "web" }` | Metadata set correctly |
| ALLOC-C024 | Sub-allocation different sizes | Various child sizes | /24, /25, /26 in same parent | All aligned correctly |

### 3.3 Create - Unhappy Path

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-C030 | Pool not found | pool_id doesn't exist | `pool_id = "missing"` | Error: "pool not found" |
| ALLOC-C031 | Parent not found | parent_cidr doesn't exist | `parent_cidr = "10.99.0.0/16"` | Error: "parent_cidr not found" |
| ALLOC-C032 | Pool is reserved | Allocate from reserved pool | Reserved pool | Error: "pool is reserved" |
| ALLOC-C033 | Parent is reserved | Sub-allocate from reserved | Reserved parent allocation | Error: "parent is a reservation" |
| ALLOC-C034 | No space in pool | Pool exhausted | All space allocated | Error: "no available block" |
| ALLOC-C035 | No space in parent | Parent exhausted | All parent space used | Error: "no available block" |
| ALLOC-C036 | Mask larger than pool | Request too large CIDR | Pool is /16, request /12 | Error: prefix larger than container |
| ALLOC-C037 | Mask larger than parent | Request too large sub-allocation | Parent is /24, request /20 | Error: prefix larger than parent |
| ALLOC-C038 | Both pool_id and parent_cidr | Specify both modes | Both attributes set | Validation error: exactly one |
| ALLOC-C039 | Neither pool_id nor parent_cidr | No allocation source | Neither attribute set | Validation error: exactly one |
| ALLOC-C040 | Missing name | No name attribute | Omit `name` | Validation error: required |
| ALLOC-C041 | Missing cidr_mask | No cidr_mask attribute | Omit `cidr_mask` | Validation error: required |
| ALLOC-C042 | Invalid status | Status not in allowed values | `status = "invalid"` | Validation error |
| ALLOC-C043 | Contiguous - no space before | No space before target | Target at start of pool | Error: no contiguous space |
| ALLOC-C044 | Contiguous - no space after | No space after target | Target at end of pool | Error: no contiguous space |
| ALLOC-C045 | Contiguous - both blocked | No adjacent space | Allocations on both sides | Error: no contiguous space |
| ALLOC-C046 | Contiguous - alignment issue | Cannot align to prefix | /16 next to /24 | Detailed alignment error |
| ALLOC-C047 | Contiguous - target not found | Target CIDR doesn't exist | `contiguous_with = "missing"` | Error: invalid target |

### 3.4 Read

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-R001 | Read allocation by ID | Allocation exists | Valid allocation ID | All attributes populated |
| ALLOC-R002 | Read sub-allocation | Child allocation exists | Sub-allocation ID | parent_cidr populated |
| ALLOC-R003 | Read reservation | Reserved allocation | Reservation ID | `status = "reservation"` |
| ALLOC-R004 | Read deleted allocation | Allocation removed | Deleted allocation | Resource removed from state |
| ALLOC-R005 | Read with metadata | Allocation has metadata | Allocation with metadata | Metadata returned |
| ALLOC-R006 | Read with contiguous_with | Allocation has constraint | Contiguous allocation | contiguous_with populated |

### 3.5 Update

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-U001 | Update metadata | Change allocation metadata | New metadata map | Metadata updated |
| ALLOC-U002 | Add metadata | Allocation had no metadata | Add metadata | Metadata added |
| ALLOC-U003 | Remove metadata | Clear all metadata | `metadata = {}` | Metadata cleared |
| ALLOC-U004 | Convert to reservation | Change status | `status = "reservation"` | Status updated |
| ALLOC-U005 | Convert from reservation | Change status back | `status = "allocation"` | Status updated |
| ALLOC-U006 | Update multiple fields | Change status and metadata | Both attributes | Both updated |

### 3.6 Update - Forces Replacement

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-U010 | Change pool_id | Attempt to change pool | Different pool_id | Plan shows destroy/create |
| ALLOC-U011 | Change parent_cidr | Attempt to change parent | Different parent_cidr | Plan shows destroy/create |
| ALLOC-U012 | Change cidr_mask | Attempt to change size | Different cidr_mask | Plan shows destroy/create |
| ALLOC-U013 | Change name | Attempt to change name | Different name | Plan shows destroy/create |
| ALLOC-U014 | Change contiguous_with | Attempt to change constraint | Different contiguous_with | Plan shows destroy/create |

### 3.7 Delete

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-D001 | Delete allocation | No children | Delete allocation | Removed from allocations.yaml |
| ALLOC-D002 | Delete already deleted | Allocation doesn't exist | Delete missing | Success (idempotent) |
| ALLOC-D003 | Delete with children | Has sub-allocations | Delete parent | Error: "has X child allocations" |
| ALLOC-D004 | Delete leaf child | Child with no grandchildren | Delete leaf | Success |
| ALLOC-D005 | Delete reservation | Reserved allocation | Delete reservation | Removed from allocations.yaml |

### 3.8 Import

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-I001 | Import by ID | Allocation exists | Import by UUID | All state populated including cidr_mask |
| ALLOC-I002 | Import non-existent | ID not found | Import missing UUID | Error: "allocation not found" |
| ALLOC-I003 | Import sub-allocation | Child allocation | Import by UUID | parent_cidr populated |
| ALLOC-I004 | Import reservation | Reserved allocation | Import by UUID | `status = "reservation"` |
| ALLOC-I005 | Import with metadata | Allocation has metadata | Import by UUID | Metadata in state |

---

## 4. Data Source Tests

### 4.1 Pools Data Source (github-ipam_pools)

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| DS-POOLS-001 | List empty pools | No pools exist | Query pools | Empty list |
| DS-POOLS-002 | List single pool | One pool exists | Query pools | List with one pool |
| DS-POOLS-003 | List multiple pools | Several pools exist | Query pools | All pools returned |
| DS-POOLS-004 | Pool attributes | Verify pool data | Query pools | pool_id, description, cidrs present |
| DS-POOLS-005 | Pool with multiple CIDRs | Pool has CIDR array | Query pool | cidrs list has all CIDRs |

### 4.2 Pool Data Source (github-ipam_pool)

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| DS-POOL-001 | Get existing pool | Pool exists | `pool_id = "existing"` | Pool data returned |
| DS-POOL-002 | Get pool with metadata | Pool has metadata | `pool_id` with metadata | metadata map populated |
| DS-POOL-003 | Pool not found | Pool doesn't exist | `pool_id = "missing"` | Error: "pool not found" |
| DS-POOL-004 | Reserved pool | Pool is reserved | `pool_id` for reserved | reserved = true |
| DS-POOL-005 | Empty pool_id | No filter provided | `pool_id = ""` | Error: validation failure |

### 4.3 Allocations Data Source (github-ipam_allocations)

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| DS-ALLOCS-001 | List all allocations | No filter | Neither pool_id nor parent_cidr | All allocations returned |
| DS-ALLOCS-002 | Filter by pool_id | Pool filter | `pool_id = "prod"` | Only prod pool allocations |
| DS-ALLOCS-003 | Filter by parent_cidr | Parent filter | `parent_cidr = "10.0.0.0/16"` | Only children of parent |
| DS-ALLOCS-004 | Empty results | No matching allocations | Filter with no matches | Empty list |
| DS-ALLOCS-005 | Pool and parent exclusive | Both filters | Both specified | Validation error |
| DS-ALLOCS-006 | Allocation attributes | Verify data | Query allocations | id, cidr, name, pool_id, parent_cidr, created_at |
| DS-ALLOCS-007 | Filter non-existent pool | Pool doesn't exist | `pool_id = "missing"` | Empty list (not error) |

### 4.4 Allocation Data Source (github-ipam_allocation)

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| DS-ALLOC-001 | Get by ID | Allocation exists | `id = "uuid..."` | Allocation data returned |
| DS-ALLOC-002 | Get by name | Allocation exists | `name = "vpc-prod"` | Allocation data returned |
| DS-ALLOC-003 | ID not found | ID doesn't exist | `id = "missing-uuid"` | Error: "allocation not found" |
| DS-ALLOC-004 | Name not found | Name doesn't exist | `name = "missing"` | Error: "allocation not found" |
| DS-ALLOC-005 | Both ID and name | Specify both | Both `id` and `name` | Validation error |
| DS-ALLOC-006 | Neither ID nor name | No filter | Neither specified | Validation error |
| DS-ALLOC-007 | Allocation with parent | Sub-allocation | Query child | parent_cidr populated |
| DS-ALLOC-008 | Allocation with metadata | Has metadata | Query with metadata | metadata map populated |

### 4.5 Next Available Data Source (github-ipam_next_available)

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| DS-NEXT-001 | Next in pool | Pool has space | `pool_id`, `cidr_mask` | Available CIDR returned |
| DS-NEXT-002 | Next in parent | Parent has space | `parent_cidr`, `cidr_mask` | Available child CIDR |
| DS-NEXT-003 | Pool exhausted | No space in pool | Full pool | Error: no available block |
| DS-NEXT-004 | Parent exhausted | No space in parent | Full parent | Error: no available block |
| DS-NEXT-005 | Pool not found | Pool doesn't exist | `pool_id = "missing"` | Error: pool not found |
| DS-NEXT-006 | Parent not found | Parent doesn't exist | `parent_cidr = "missing"` | Error: parent not found |
| DS-NEXT-007 | Both pool and parent | Both specified | Both filters | Validation error |
| DS-NEXT-008 | Neither pool nor parent | No filter | Neither specified | Validation error |
| DS-NEXT-009 | Invalid cidr_mask | Out of range | `cidr_mask = 200` | Validation error |
| DS-NEXT-010 | Mask larger than container | Request too large | Pool /16, mask /8 | Error: prefix too large |
| DS-NEXT-011 | Consistent results | Multiple calls | Same inputs | Same CIDR each time |
| DS-NEXT-012 | Reserved pool | Pool is reserved | Reserved pool_id | Depends on implementation |

---

## 5. Client/API Tests

### 5.1 HTTP Status Code Handling

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| API-001 | Success (200) | Normal API response | Any valid operation | Operation succeeds |
| API-002 | Created (201) | File created | Create new file | Operation succeeds |
| API-003 | Not Found (404) | File doesn't exist | Read missing file | Empty data returned or error |
| API-004 | Conflict (409) | SHA mismatch | Update with stale SHA | Retry triggered |
| API-005 | Rate Limited (429) | Too many requests | Burst of requests | Retry with backoff |
| API-006 | Unauthorized (401) | Invalid token | Bad token | Error: authentication failed |
| API-007 | Forbidden (403) | No permission | No repo access | Error: forbidden |
| API-008 | Server Error (500) | GitHub error | Internal error | Error: server error |
| API-009 | Service Unavailable (503) | GitHub down | Outage | Error with potential retry |

### 5.2 Conflict Resolution (409)

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| CONF-001 | Single conflict - resolved | One retry needed | First attempt 409, second 200 | Success after retry |
| CONF-002 | Multiple conflicts | Several retries | Three 409s, then 200 | Success after retries |
| CONF-003 | Max retries exceeded | Too many conflicts | 409 for all attempts | Error: max retries exceeded |
| CONF-004 | Conflict on pool create | Pool creation conflict | Concurrent pool create | One succeeds, one retries |
| CONF-005 | Conflict on allocation | Allocation conflict | Concurrent allocations | Both eventually succeed |
| CONF-006 | Conflict during delete | Delete conflict | Concurrent delete | Success (idempotent) |
| CONF-007 | Exponential backoff | Verify delay increases | Multiple 409s | Delays: 200ms, 400ms, 800ms... |
| CONF-008 | Jitter applied | Verify randomization | Multiple retries | Delays have ±50% variation |
| CONF-009 | Max delay capped | Delay doesn't exceed cap | Many retries | Delay caps at 5 seconds |

### 5.3 Rate Limiting (429)

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| RATE-001 | Single rate limit | One 429 response | Single 429, then 200 | Success after backoff |
| RATE-002 | Respect Retry-After | Header present | 429 with Retry-After: 30 | Wait 30+ seconds |
| RATE-003 | No Retry-After | Header missing | 429 without header | Use default backoff |
| RATE-004 | Extended rate limit | Many 429s | Several 429s | Continue retrying |
| RATE-005 | Rate limit during read | Read operation limited | 429 on read | Retry and succeed |
| RATE-006 | Rate limit during write | Write operation limited | 429 on write | Retry with backoff |

### 5.4 Timeout Handling

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| TIME-001 | Context timeout | Operation times out | Context with deadline | Error: context deadline exceeded |
| TIME-002 | Network timeout | Connection timeout | Slow network | Error: timeout |
| TIME-003 | Timeout during retry | Timeout while retrying | Timeout after retries | Error: timeout |
| TIME-004 | Partial response timeout | Slow response | Response takes too long | Error: timeout |

### 5.5 Network Errors

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| NET-001 | Connection refused | Cannot connect | No network | Error: connection refused |
| NET-002 | DNS resolution failure | Bad hostname | Invalid host | Error: DNS lookup failed |
| NET-003 | Connection reset | Connection dropped | Mid-request drop | Error: connection reset |
| NET-004 | SSL/TLS error | Certificate issue | Bad cert | Error: TLS handshake failed |

### 5.6 File Operations

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| FILE-001 | Read existing file | File exists | Read pools.yaml | Content and SHA returned |
| FILE-002 | Read missing file | File doesn't exist | Read missing file | Empty content, no SHA |
| FILE-003 | Write new file | Create file | Write to new path | File created |
| FILE-004 | Update existing file | Modify file | Write with SHA | File updated |
| FILE-005 | Update with wrong SHA | Stale SHA | Write with old SHA | 409 conflict |
| FILE-006 | Delete file | Remove file | Delete with SHA | File deleted |
| FILE-007 | Invalid JSON | Malformed content | Parse bad JSON | Error: parse failure |
| FILE-008 | Invalid YAML | Malformed content | Parse bad YAML | Error: parse failure |

---

## 6. IPAM Allocator Tests

### 6.1 FindNextAvailableInPool

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-FN001 | Empty pool | First allocation | No existing allocations | First block in pool |
| ALLOC-FN002 | First block taken | Second allocation | 10.0.0.0/16 exists | 10.1.0.0/16 |
| ALLOC-FN003 | Find gap | Gap between allocations | 10.0.0.0/16 and 10.2.0.0/16 | 10.1.0.0/16 |
| ALLOC-FN004 | Align to boundary | Unaligned start | Request /16 after /17 | Next /16 boundary |
| ALLOC-FN005 | Pool exhausted | No space | Full pool | Error |
| ALLOC-FN006 | Multiple pool CIDRs | Pool has array | First CIDR full | Allocate from second |
| ALLOC-FN007 | Skip invalid allocations | Bad CIDR in DB | Invalid allocation entry | Skip and continue |
| ALLOC-FN008 | Different sizes | Various masks | /24, /20, /28 requests | Correctly aligned |
| ALLOC-FN009 | Edge - start of range | Allocate at boundary | First possible block | Block at start |
| ALLOC-FN010 | Edge - end of range | Allocate at end | Only space at end | Block at end |

### 6.2 FindNextAvailableInParent

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-FP001 | Empty parent | First child | No children | First block in parent |
| ALLOC-FP002 | Sequential children | Multiple children | Three /24s in /16 | Sequential CIDRs |
| ALLOC-FP003 | Fill gap | Gap in children | Gap between /24s | CIDR in gap |
| ALLOC-FP004 | Parent exhausted | Full parent | All space used | Error |
| ALLOC-FP005 | Different child sizes | Various masks | /25, /26, /27 | Correctly sized |
| ALLOC-FP006 | Boundary alignment | Align children | /26 request | Aligned to /26 boundary |

### 6.3 Contiguous Allocation

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-CT001 | Before target | Space before | Target not at start | Block before target |
| ALLOC-CT002 | After target | Space after | Target not at end | Block after target |
| ALLOC-CT003 | No space before | Target at start | Target = pool start | Block after target |
| ALLOC-CT004 | No space after | Target at end | Target = pool end | Block before target |
| ALLOC-CT005 | Both sides taken | No adjacent space | Allocations on both sides | Error with details |
| ALLOC-CT006 | Different sizes | Size mismatch | /16 contiguous with /24 | Alignment handled |
| ALLOC-CT007 | Target outside pool | Invalid target | Target not in pool | Error |
| ALLOC-CT008 | Target not found | Target doesn't exist | Non-existent CIDR | Error |

### 6.4 Overlap Detection

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-OV001 | No overlap | Disjoint networks | 10.0.0.0/16 vs 10.1.0.0/16 | false |
| ALLOC-OV002 | Exact overlap | Same network | 10.0.0.0/16 vs 10.0.0.0/16 | true |
| ALLOC-OV003 | Contained | One contains other | 10.0.0.0/8 vs 10.1.0.0/16 | true |
| ALLOC-OV004 | Contains | One contains other | 10.1.0.0/16 vs 10.0.0.0/8 | true |
| ALLOC-OV005 | Partial overlap | Ranges intersect | 10.0.0.0/15 vs 10.1.0.0/16 | true |
| ALLOC-OV006 | Adjacent no overlap | Touching but disjoint | 10.0.0.0/16 vs 10.1.0.0/16 | false |

### 6.5 Available Space Calculation

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| ALLOC-SP001 | Empty container | No allocations | Empty pool | Total addresses |
| ALLOC-SP002 | Partially filled | Some allocations | Half allocated | Half addresses |
| ALLOC-SP003 | Fully allocated | All space used | Full pool | 0 addresses |
| ALLOC-SP004 | Various sizes | Mixed allocations | /24 + /25 + /26 | Correct sum |
| ALLOC-SP005 | Skip invalid | Invalid allocations | Bad CIDR entry | Skip in calculation |

---

## 7. README Generator Tests

### 7.1 Main README Generation

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| README-001 | Empty state | No pools or allocations | Empty data | Valid README with empty tables |
| README-002 | Single pool | One pool | One pool definition | Pool in table |
| README-003 | Multiple pools | Several pools | Multiple pools | All pools in tables |
| README-004 | Pool with allocations | Pool has children | Pool with allocations | Utilization shown |
| README-005 | Reserved pool | Pool is reserved | Reserved pool | 🟠 icon shown |
| README-006 | Available gap | Unallocated space | Gap between pools | ⚪ Available row |
| README-007 | Class A section | 10.x pools | Class A pools | Correct section |
| README-008 | Class B section | 172.16.x pools | Class B pools | Correct section |
| README-009 | Class C section | 192.168.x pools | Class C pools | Correct section |
| README-010 | HCL code blocks | Terraform examples | Pools defined | Valid HCL examples |
| README-011 | Totals row | Summary stats | Various pools | Correct totals |
| README-012 | Warning quote | Auto-gen warning | Any state | Warning at top |
| README-013 | Legend table | Status legend | Any state | Legend with icons |

### 7.2 Pool Page Generation

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| README-P001 | Pool with no allocations | Empty pool | No allocations | "No allocations yet" |
| README-P002 | Pool with allocations | Has children | Multiple allocations | Allocations table |
| README-P003 | Pool metadata | Has metadata | Pool with metadata | Metadata in overview |
| README-P004 | Reserved pool | Pool reserved | Reserved pool | Reserved banner |
| README-P005 | Allocation HCL | Terraform examples | Allocations | Valid HCL |
| README-P006 | Back link | Navigation | Any pool | Link to overview |
| README-P007 | Reserved allocation | Reserved child | Reservation in pool | 🟠 icon |
| README-P008 | CIDR with range | Address range | Any allocation | CIDR (start - end) |

### 7.3 Formatting

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| README-F001 | Number formatting | Large numbers | 1048576 addresses | "1,048,576" |
| README-F002 | Percentage formatting | Utilization percent | 62.5% | "62.5%" |
| README-F003 | Utilization bar | Visual bar | 50% utilization | "█████░░░░░" |
| README-F004 | CIDR range format | IP range | 10.0.0.0/16 | "10.0.0.0 - 10.0.255.255" |
| README-F005 | Metadata title case | Hyphen keys | "env-name" | "Env Name" |
| README-F006 | Icon spacing | Non-breaking space | Status icons | "🔵&nbsp;&nbsp;Allocated" |

---

## 8. Integration Tests

### 8.1 End-to-End Workflows

| Test ID | Test Name | Description | Steps | Expected Result |
|---------|-----------|-------------|-------|-----------------|
| INT-001 | Create pool and allocation | Full workflow | Create pool → Create allocation | Both exist correctly |
| INT-002 | Hierarchical allocations | Three-level hierarchy | Pool → VPC → Subnet | All linked correctly |
| INT-003 | Pool lifecycle | Full CRUD | Create → Update → Delete | Clean state |
| INT-004 | Allocation lifecycle | Full CRUD | Create → Update → Delete | Clean state |
| INT-005 | Import existing | Import workflow | Manual create → Import | State matches |
| INT-006 | Concurrent creates | Race condition | Two parallel creates | Both succeed |
| INT-007 | Destroy infrastructure | Full teardown | Create all → Destroy all | Empty state |
| INT-008 | Plan accuracy | Plan vs apply | Plan → Apply | Actual matches planned |
| INT-009 | Refresh accuracy | State refresh | Modify files → Refresh | State updated |
| INT-010 | Data source after create | Query new resources | Create → Query data source | Data matches |

### 8.2 Cross-Resource Interactions

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| INT-020 | Allocation depends on pool | Implicit dependency | Allocation references pool | Correct order |
| INT-021 | Sub-allocation depends on parent | Implicit dependency | Child references parent | Correct order |
| INT-022 | Delete order | Reverse dependencies | Delete all | Children before parents |
| INT-023 | Pool delete blocked | Allocations exist | Delete pool with children | Error |
| INT-024 | Cascade awareness | Manual child delete | Delete child, then parent | Success |

### 8.3 State Management

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| INT-030 | Drift detection | External change | Modify file, run plan | Drift detected |
| INT-031 | State file consistency | Multiple applies | Several applies | Consistent state |
| INT-032 | Orphan cleanup | Delete from file | Remove from YAML | Resource removed from state |
| INT-033 | ID stability | Multiple applies | Same config | Same IDs |

---

## 9. Performance Tests

### 9.1 Scalability

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| PERF-001 | Many pools | 100 pools | Create 100 pools | Completes in reasonable time |
| PERF-002 | Many allocations | 1000 allocations | Create 1000 allocations | Completes in reasonable time |
| PERF-003 | Deep hierarchy | 5-level nesting | Nested sub-allocations | Handles depth |
| PERF-004 | Large metadata | Big metadata maps | 100 metadata keys | Handles size |
| PERF-005 | Allocation search | Find in large DB | Search 1000 allocations | Fast lookup |

### 9.2 Retry Performance

| Test ID | Test Name | Description | Input | Expected Output |
|---------|-----------|-------------|-------|-----------------|
| PERF-010 | Max retry duration | Worst case retries | 10 retries with max backoff | Total time reasonable |
| PERF-011 | Backoff calculation | Verify formula | Multiple attempts | Correct delays |
| PERF-012 | Jitter distribution | Verify randomization | Many retries | Even distribution |

---

## Test Implementation Guidelines

### Unit Test Structure

```go
func TestPoolResource_Create_HappyPath(t *testing.T) {
    t.Parallel()

    testCases := []struct {
        name         string
        config       PoolResourceModel
        existingData *PoolsConfig
        expected     string
        expectError  bool
    }{
        // Test cases from table above
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Setup mock client
            // Execute operation
            // Verify results
        })
    }
}
```

### Mock Client Interface

```go
type MockGitHubClient struct {
    GetPoolsFunc        func(ctx context.Context) (*PoolsConfig, error)
    GetPoolsWithSHAFunc func(ctx context.Context) (*PoolsConfig, string, error)
    UpdatePoolsFunc     func(ctx context.Context, pools *PoolsConfig, sha, msg string) error
    // ... other methods

    // Control responses
    ConflictCount int  // Number of 409s before success
    RateLimitCount int // Number of 429s before success
}
```

### Acceptance Test Tags

```go
// +build acceptance

func TestAccPoolResource_Create(t *testing.T) {
    // Real GitHub API tests
    // Requires GITHUB_TOKEN, test repo
}
```

---

## Coverage Requirements

| Component | Minimum Coverage |
|-----------|-----------------|
| Pool Resource | 90% |
| Allocation Resource | 90% |
| Data Sources | 85% |
| Client | 80% |
| Allocator | 95% |
| README Generator | 80% |
| Retry Logic | 90% |

---

## Running Tests

```bash
# Unit tests
go test ./internal/... -v

# With coverage
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Acceptance tests (requires GitHub credentials)
GITHUB_TOKEN=xxx TF_ACC=1 go test ./internal/... -v -run TestAcc

# Specific test
go test ./internal/resources -v -run TestPoolResource_Create

# Race detection
go test ./internal/... -race
```

---

## Appendix: Test Data Fixtures

### Empty State
```yaml
# pools.yaml
pools: {}
```

```json
// allocations.yaml
{
  "version": "1.0",
  "allocations": {}
}
```

### Sample Pool Configuration
```yaml
pools:
  production:
    cidr:
      - "10.0.0.0/12"
    description: "Production IP pool"
    metadata:
      environment: "production"
    reserved: false

  reserved-future:
    cidr:
      - "10.128.0.0/10"
    description: "Reserved for future use"
    reserved: true
```

### Sample Allocations
```json
{
  "version": "1.0",
  "allocations": {
    "production": [
      {
        "cidr": "10.0.0.0/16",
        "id": "550e8400-e29b-41d4-a716-446655440000",
        "name": "vpc-prod-east",
        "parent_cidr": null,
        "metadata": {"region": "us-east-1"},
        "created_at": "2024-01-15T10:30:00Z",
        "reserved": false
      },
      {
        "cidr": "10.0.0.0/24",
        "id": "550e8400-e29b-41d4-a716-446655440001",
        "name": "subnet-prod-web",
        "parent_cidr": "10.0.0.0/16",
        "metadata": {"tier": "web"},
        "created_at": "2024-01-15T10:35:00Z",
        "reserved": false
      }
    ]
  }
}
```

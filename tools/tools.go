//go:build tools
// +build tools

// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

// This file declares tool dependencies that are used for development but not
// compiled into the final binary.

package tools

import (
	// Documentation generation
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

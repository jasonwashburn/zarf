// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The Zarf Authors

// Package lint contains functions for verifying zarf yaml files are valid
package lint

import (
	"testing"

	"github.com/defenseunicorns/zarf/src/types"
	"github.com/stretchr/testify/require"
)

func TestGroupFindingsByPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		findings    []types.PackageError
		severity    types.Severity
		packageName string
		want        map[string][]types.PackageError
	}{
		{
			name: "same package multiple findings",
			findings: []types.PackageError{
				{Category: types.SevWarn, PackageNameOverride: "import", PackagePathOverride: "path"},
				{Category: types.SevWarn, PackageNameOverride: "import", PackagePathOverride: "path"},
			},
			severity:    types.SevWarn,
			packageName: "testPackage",
			want: map[string][]types.PackageError{
				"path": {
					{Category: types.SevWarn, PackageNameOverride: "import", PackagePathOverride: "path"},
					{Category: types.SevWarn, PackageNameOverride: "import", PackagePathOverride: "path"},
				},
			},
		},
		{
			name: "different packages single finding",
			findings: []types.PackageError{
				{Category: types.SevWarn, PackageNameOverride: "import", PackagePathOverride: "path"},
				{Category: types.SevErr, PackageNameOverride: "", PackagePathOverride: ""},
			},
			severity:    types.SevWarn,
			packageName: "testPackage",
			want: map[string][]types.PackageError{
				"path": {{Category: types.SevWarn, PackageNameOverride: "import", PackagePathOverride: "path"}},
				".":    {{Category: types.SevErr, PackageNameOverride: "testPackage", PackagePathOverride: "."}},
			},
		},
		{
			name: "Multiple findings, mixed severity",
			findings: []types.PackageError{
				{Category: types.SevWarn, PackageNameOverride: "", PackagePathOverride: ""},
				{Category: types.SevErr, PackageNameOverride: "", PackagePathOverride: ""},
			},
			severity:    types.SevErr,
			packageName: "testPackage",
			want: map[string][]types.PackageError{
				".": {{Category: types.SevErr, PackageNameOverride: "testPackage", PackagePathOverride: "."}},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, groupFindingsByPath(tt.findings, tt.severity, tt.packageName))
		})
	}
}

func TestValidator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		severity types.Severity
		expected bool
		findings []types.PackageError
	}{
		{
			findings: []types.PackageError{
				{
					Category: types.SevErr,
				},
			},
			severity: types.SevErr,
			expected: true,
		},
		{
			findings: []types.PackageError{
				{
					Category: types.SevWarn,
				},
			},
			severity: types.SevWarn,
			expected: true,
		},
		{
			findings: []types.PackageError{
				{
					Category: types.SevWarn,
				},
				{
					Category: types.SevErr,
				},
			},
			severity: types.SevErr,
			expected: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run("test has severity", func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, hasSeverity(tc.findings, tc.severity))
		})
	}
}

// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The Zarf Authors

// Package lint contains functions for verifying zarf yaml files are valid
package lint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/zarf/src/pkg/packager/composer"
	"github.com/defenseunicorns/zarf/src/pkg/variables"
	"github.com/defenseunicorns/zarf/src/types"
	goyaml "github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
)

// When we want to test the absence of a field we can't do it through a struct
// for non pointer fields since it will be auto initialized
const badZarfPackage = `
kind: ZarfInitConfig
metadata:
  name: invalid-name
  description: Testing bad yaml

components:
- name: import-test
  import:
    path: 123123
  charts:
  - noWait: true
  manifests:
  - namespace: no-name-for-manifest
`

const goodZarfPackage = `
x-name: &name good-zarf-package

kind: ZarfPackageConfig
metadata:
  name: *name
  x-description: Testing good yaml with yaml extension

components:
  - name: baseline
    required: true
    x-foo: bar

`

func readAndUnmarshalYaml[T interface{}](t *testing.T, yamlString string) T {
	t.Helper()
	var unmarshalledYaml T
	err := goyaml.Unmarshal([]byte(yamlString), &unmarshalledYaml)
	if err != nil {
		t.Errorf("error unmarshalling yaml: %v", err)
	}
	return unmarshalledYaml
}

// TODO t.parallel everything
func TestValidateSchema(t *testing.T) {
	getZarfSchema := func(t *testing.T) []byte {
		t.Helper()
		file, err := os.ReadFile("../../../../zarf.schema.json")
		if err != nil {
			t.Errorf("error reading file: %v", err)
		}
		return file
	}

	tests := []struct {
		name                  string
		pkg                   types.ZarfPackage
		expectedSchemaStrings []string
	}{
		{
			name: "valid package",
			pkg: types.ZarfPackage{
				Kind: types.ZarfInitConfig,
				Metadata: types.ZarfMetadata{
					Name: "valid-name",
				},
				Components: []types.ZarfComponent{
					{
						Name: "valid-comp",
					},
				},
			},
			expectedSchemaStrings: nil,
		},
		{
			name: "no comp or kind",
			pkg: types.ZarfPackage{
				Metadata: types.ZarfMetadata{
					Name: "no-comp-or-kind",
				},
				Components: []types.ZarfComponent{},
			},
			expectedSchemaStrings: []string{
				"kind: kind must be one of the following: \"ZarfInitConfig\", \"ZarfPackageConfig\"",
				"components: Array must have at least 1 items",
			},
		},
		{
			name: "invalid package",
			pkg: types.ZarfPackage{
				Kind: types.ZarfInitConfig,
				Metadata: types.ZarfMetadata{
					Name: "-invalid-name",
				},
				Components: []types.ZarfComponent{
					{
						Name: "invalid-name",
						Only: types.ZarfComponentOnlyTarget{
							LocalOS: "unsupportedOS",
						},
					},
					{
						Name: "actions",
						Actions: types.ZarfComponentActions{
							OnCreate: types.ZarfComponentActionSet{
								Before: []types.ZarfComponentAction{
									{
										Cmd:          "echo 'invalid setVariable'",
										SetVariables: []variables.Variable{{Name: "not_uppercase"}},
									},
								},
							},
							OnRemove: types.ZarfComponentActionSet{
								OnSuccess: []types.ZarfComponentAction{
									{
										Cmd:          "echo 'invalid setVariable'",
										SetVariables: []variables.Variable{{Name: "not_uppercase"}},
									},
								},
							},
						},
					},
				},
				Variables: []variables.InteractiveVariable{
					{
						Variable: variables.Variable{Name: "not_uppercase"},
					},
				},
				Constants: []variables.Constant{
					{
						Name: "not_uppercase",
					},
				},
			},
			expectedSchemaStrings: []string{
				"metadata.name: Does not match pattern '^[a-z0-9][a-z0-9\\-]*$'",
				"variables.0.name: Does not match pattern '^[A-Z0-9_]+$'",
				"constants.0.name: Does not match pattern '^[A-Z0-9_]+$'",
				"components.0.only.localOS: components.0.only.localOS must be one of the following: \"linux\", \"darwin\", \"windows\"",
				"components.1.actions.onCreate.before.0.setVariables.0.name: Does not match pattern '^[A-Z0-9_]+$'",
				"components.1.actions.onRemove.onSuccess.0.setVariables.0.name: Does not match pattern '^[A-Z0-9_]+$'",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schemaErrs, err := runSchema(getZarfSchema(t), tt.pkg)
			require.NoError(t, err)
			var schemaStrings []string
			for _, schemaErr := range schemaErrs {
				schemaStrings = append(schemaStrings, schemaErr.String())
			}
			require.ElementsMatch(t, tt.expectedSchemaStrings, schemaStrings)
		})
	}

	t.Run("validate schema fail with errors not possible from object", func(t *testing.T) {
		unmarshalledYaml := readAndUnmarshalYaml[interface{}](t, badZarfPackage)
		schemaErrs, err := runSchema(getZarfSchema(t), unmarshalledYaml)
		require.NoError(t, err)
		var schemaStrings []string
		for _, schemaErr := range schemaErrs {
			schemaStrings = append(schemaStrings, schemaErr.String())
		}
		expectedSchemaStrings := []string{
			"components.0.import.path: Invalid type. Expected: string, given: integer",
			"components.0.charts.0: name is required",
			"components.0.charts.0: namespace is required",
			"components.0.charts.0: version is required",
			"components.0.manifests.0: name is required",
		}

		require.ElementsMatch(t, expectedSchemaStrings, schemaStrings)
	})
}

func TestValidateComponent(t *testing.T) {

	// Make this an object instead of a yaml string
	t.Run("Template in component import success", func(t *testing.T) {
		unmarshalledYaml := readAndUnmarshalYaml[types.ZarfPackage](t, goodZarfPackage)
		validator := Validator{typedZarfPackage: unmarshalledYaml}
		for _, component := range validator.typedZarfPackage.Components {
			lintComponent(&validator, &composer.Node{ZarfComponent: component})
		}
		require.Empty(t, validator.findings)
	})

	t.Run("Path template in component import failure", func(t *testing.T) {
		pathVar := "###ZARF_PKG_TMPL_PATH###"
		pathComponent := types.ZarfComponent{Import: types.ZarfComponentImport{Path: pathVar}}
		validator := Validator{typedZarfPackage: types.ZarfPackage{Components: []types.ZarfComponent{pathComponent}}}
		checkForVarInComponentImport(&validator, &composer.Node{ZarfComponent: pathComponent})
		require.Equal(t, pathVar, validator.findings[0].item)
	})

	t.Run("OCI template in component import failure", func(t *testing.T) {
		ociPathVar := "oci://###ZARF_PKG_TMPL_PATH###"
		URLComponent := types.ZarfComponent{Import: types.ZarfComponentImport{URL: ociPathVar}}
		validator := Validator{typedZarfPackage: types.ZarfPackage{Components: []types.ZarfComponent{URLComponent}}}
		checkForVarInComponentImport(&validator, &composer.Node{ZarfComponent: URLComponent})
		require.Equal(t, ociPathVar, validator.findings[0].item)
	})

	t.Run("Unpinnned repo warning", func(t *testing.T) {
		validator := Validator{}
		unpinnedRepo := "https://github.com/defenseunicorns/zarf-public-test.git"
		component := types.ZarfComponent{Repos: []string{
			unpinnedRepo,
			"https://dev.azure.com/defenseunicorns/zarf-public-test/_git/zarf-public-test@v0.0.1"}}
		checkForUnpinnedRepos(&validator, &composer.Node{ZarfComponent: component})
		require.Equal(t, unpinnedRepo, validator.findings[0].item)
		require.Len(t, validator.findings, 1)
	})

	t.Run("Unpinnned image warning", func(t *testing.T) {
		validator := Validator{}
		unpinnedImage := "registry.com:9001/whatever/image:1.0.0"
		badImage := "badimage:badimage@@sha256:3fbc632167424a6d997e74f5"
		component := types.ZarfComponent{Images: []string{
			unpinnedImage,
			"busybox:latest@sha256:3fbc632167424a6d997e74f52b878d7cc478225cffac6bc977eedfe51c7f4e79",
			badImage}}
		checkForUnpinnedImages(&validator, &composer.Node{ZarfComponent: component})
		require.Equal(t, unpinnedImage, validator.findings[0].item)
		require.Equal(t, badImage, validator.findings[1].item)
		require.Len(t, validator.findings, 2)
	})

	t.Run("Unpinnned file warning", func(t *testing.T) {
		validator := Validator{}
		fileURL := "http://example.com/file.zip"
		localFile := "local.txt"
		zarfFiles := []types.ZarfFile{
			{
				Source: fileURL,
			},
			{
				Source: localFile,
			},
			{
				Source: fileURL,
				Shasum: "fake-shasum",
			},
		}
		component := types.ZarfComponent{Files: zarfFiles}
		checkForUnpinnedFiles(&validator, &composer.Node{ZarfComponent: component})
		require.Equal(t, fileURL, validator.findings[0].item)
		require.Len(t, validator.findings, 1)
	})

	t.Run("Wrap standalone numbers in bracket", func(t *testing.T) {
		input := "components12.12.import.path"
		expected := ".components12.[12].import.path"
		actual := makeFieldPathYqCompat(input)
		require.Equal(t, expected, actual)
	})

	t.Run("root doesn't change", func(t *testing.T) {
		input := "(root)"
		actual := makeFieldPathYqCompat(input)
		require.Equal(t, input, actual)
	})

	t.Run("Test composable components", func(t *testing.T) {
		pathVar := "fake-path"
		unpinnedImage := "unpinned:latest"
		pathComponent := types.ZarfComponent{
			Import: types.ZarfComponentImport{Path: pathVar},
			Images: []string{unpinnedImage}}
		validator := Validator{
			typedZarfPackage: types.ZarfPackage{Components: []types.ZarfComponent{pathComponent},
				Metadata: types.ZarfMetadata{Name: "test-zarf-package"}}}

		createOpts := types.ZarfCreateOptions{Flavor: "", BaseDir: "."}
		lintComponents(&validator, &createOpts)
		// Require.contains rather than equals since the error message changes from linux to windows
		require.Contains(t, validator.findings[0].description, fmt.Sprintf("open %s", filepath.Join("fake-path", "zarf.yaml")))
		require.Equal(t, ".components.[0].import.path", validator.findings[0].yqPath)
		require.Equal(t, ".", validator.findings[0].packageRelPath)
		require.Equal(t, unpinnedImage, validator.findings[1].item)
		require.Equal(t, ".", validator.findings[1].packageRelPath)
	})

	t.Run("isImagePinned", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			input    string
			expected bool
			err      error
		}{
			{
				input:    "registry.com:8080/defenseunicorns/whatever",
				expected: false,
				err:      nil,
			},
			{
				input:    "ghcr.io/defenseunicorns/pepr/controller:v0.15.0",
				expected: false,
				err:      nil,
			},
			{
				input:    "busybox:latest@sha256:3fbc632167424a6d997e74f52b878d7cc478225cffac6bc977eedfe51c7f4e79",
				expected: true,
				err:      nil,
			},
			{
				input:    "busybox:bad/image",
				expected: false,
				err:      errors.New("invalid reference format"),
			},
			{
				input:    "busybox:###ZARF_PKG_TMPL_BUSYBOX_IMAGE###",
				expected: true,
				err:      nil,
			},
		}
		for _, tc := range tests {
			t.Run(tc.input, func(t *testing.T) {
				actual, err := isPinnedImage(tc.input)
				if err != nil {
					require.EqualError(t, err, tc.err.Error())
				}
				require.Equal(t, tc.expected, actual)
			})
		}
	})
}

func TestValidator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		validator Validator
		severity  category
		expected  bool
	}{
		{
			validator: Validator{findings: []validatorMessage{
				{
					category:    categoryError,
					description: "1 error",
				},
			}},
			severity: categoryError,
			expected: true,
		},
		{
			validator: Validator{findings: []validatorMessage{
				{
					category:    categoryWarning,
					description: "1 error",
				},
			}},
			severity: categoryError,
			expected: false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run("test has severity", func(t *testing.T) {
			t.Parallel()
			tc.validator.hasSeverity(categoryError)
		})
	}
}

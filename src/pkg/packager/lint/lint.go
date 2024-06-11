// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The Zarf Authors

// Package lint contains functions for verifying zarf yaml files are valid
package lint

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/config/lang"
	"github.com/defenseunicorns/zarf/src/pkg/layout"
	"github.com/defenseunicorns/zarf/src/pkg/packager/composer"
	"github.com/defenseunicorns/zarf/src/pkg/transform"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/types"
	"github.com/xeipuuv/gojsonschema"
)

// FileLoader is an interface for reading files, it decouples the lint package from the go embed package
type FileLoader interface {
	ReadFile(path string) ([]byte, error)
}

// ZarfSchema is exported so main.go can embed the schema file
var ZarfSchema FileLoader

// Validate validates a zarf file against the zarf schema, returns *validator with warnings or errors if they exist
// along with an error if the validation itself failed
func Validate(ctx context.Context, pp *layout.PackagePaths, createOpts types.ZarfCreateOptions) (*Validator, error) {
	validator := &Validator{baseDir: createOpts.BaseDir}

	if err := utils.ReadYaml(pp.ZarfYAML, &validator.zarfPackage); err != nil {
		return nil, err
	}

	if err := lintComponents(ctx, validator, &createOpts); err != nil {
		return nil, err
	}

	jsonSchema, err := ZarfSchema.ReadFile("zarf.schema.json")
	if err != nil {
		return nil, err
	}

	validator.zarfPackage.Metadata.Architecture = config.GetArch(validator.zarfPackage.Metadata.Architecture)
	composed, _, err := composer.ComposeComponents(ctx, validator.zarfPackage, createOpts.Flavor)
	if err != nil {
		return nil, err
	}

	var untypedZarfPackage map[string]interface{}
	if err := utils.ReadYaml(pp.ZarfYAML, &untypedZarfPackage); err != nil {
		return nil, err
	}
	untypedZarfPackage["components"] = composed.Components

	if err = validateSchema(validator, jsonSchema, untypedZarfPackage); err != nil {
		return nil, err
	}

	return validator, nil
}

func lintComponents(ctx context.Context, validator *Validator, createOpts *types.ZarfCreateOptions) error {
	for i, component := range validator.zarfPackage.Components {
		arch := config.GetArch(validator.zarfPackage.Metadata.Architecture)
		if !composer.CompatibleComponent(component, arch, createOpts.Flavor) {
			continue
		}

		chain, err := composer.NewImportChain(ctx, component, i, validator.zarfPackage.Metadata.Name, arch, createOpts.Flavor)
		baseComponent := chain.Head()

		var badImportYqPath string
		if baseComponent != nil {
			if baseComponent.Import.URL != "" {
				badImportYqPath = fmt.Sprintf(".components.[%d].import.url", i)
			}
			if baseComponent.Import.Path != "" {
				badImportYqPath = fmt.Sprintf(".components.[%d].import.path", i)
			}
		}
		if err != nil {
			validator.addError(validatorMessage{
				description:    err.Error(),
				packageRelPath: ".",
				packageName:    validator.zarfPackage.Metadata.Name,
				yqPath:         badImportYqPath,
			})
		}

		node := baseComponent
		for node != nil {
			checkForVarInComponentImport(validator, node)
			if err = fillComponentTemplate(validator, node, createOpts); err != nil {
				return err
			}
			lintComponent(validator, node)
			node = node.Next()
		}
	}
	return nil
}

func fillComponentTemplate(validator *Validator, node *composer.Node, createOpts *types.ZarfCreateOptions) error {
	templateMap := map[string]string{}

	setVarsAndWarn := func(templatePrefix string, deprecated bool) {
		yamlTemplates, err := utils.FindYamlTemplates(node, templatePrefix, "###")
		if err != nil {
			validator.addWarning(validatorMessage{
				description:    err.Error(),
				packageRelPath: node.ImportLocation(),
				packageName:    node.OriginalPackageName(),
			})
		}

		for key := range yamlTemplates {
			if deprecated {
				validator.addWarning(validatorMessage{
					description:    fmt.Sprintf(lang.PkgValidateTemplateDeprecation, key, key, key),
					packageRelPath: node.ImportLocation(),
					packageName:    node.OriginalPackageName(),
				})
			}
			_, present := createOpts.SetVariables[key]
			if !present {
				validator.addWarning(validatorMessage{
					description:    lang.UnsetVarLintWarning,
					packageRelPath: node.ImportLocation(),
					packageName:    node.OriginalPackageName(),
				})
			}
		}
		for key, value := range createOpts.SetVariables {
			templateMap[fmt.Sprintf("%s%s###", templatePrefix, key)] = value
		}
	}

	setVarsAndWarn(types.ZarfPackageTemplatePrefix, false)

	// [DEPRECATION] Set the Package Variable syntax as well for backward compatibility
	setVarsAndWarn(types.ZarfPackageVariablePrefix, true)

	return utils.ReloadYamlTemplate(node, templateMap)
}

func isPinnedImage(image string) (bool, error) {
	transformedImage, err := transform.ParseImageRef(image)
	if err != nil {
		if strings.Contains(image, types.ZarfPackageTemplatePrefix) ||
			strings.Contains(image, types.ZarfPackageVariablePrefix) {
			return true, nil
		}
		return false, err
	}
	return (transformedImage.Digest != ""), err
}

func isPinnedRepo(repo string) bool {
	return (strings.Contains(repo, "@"))
}

func lintComponent(validator *Validator, node *composer.Node) {
	checkForUnpinnedRepos(validator, node)
	checkForUnpinnedImages(validator, node)
	checkForUnpinnedFiles(validator, node)
}

func checkForUnpinnedRepos(validator *Validator, node *composer.Node) {
	for j, repo := range node.Repos {
		repoYqPath := fmt.Sprintf(".components.[%d].repos.[%d]", node.Index(), j)
		if !isPinnedRepo(repo) {
			validator.addWarning(validatorMessage{
				yqPath:         repoYqPath,
				packageRelPath: node.ImportLocation(),
				packageName:    node.OriginalPackageName(),
				description:    "Unpinned repository",
				item:           repo,
			})
		}
	}
}

func checkForUnpinnedImages(validator *Validator, node *composer.Node) {
	for j, image := range node.Images {
		imageYqPath := fmt.Sprintf(".components.[%d].images.[%d]", node.Index(), j)
		pinnedImage, err := isPinnedImage(image)
		if err != nil {
			validator.addError(validatorMessage{
				yqPath:         imageYqPath,
				packageRelPath: node.ImportLocation(),
				packageName:    node.OriginalPackageName(),
				description:    "Invalid image reference",
				item:           image,
			})
			continue
		}
		if !pinnedImage {
			validator.addWarning(validatorMessage{
				yqPath:         imageYqPath,
				packageRelPath: node.ImportLocation(),
				packageName:    node.OriginalPackageName(),
				description:    "Image not pinned with digest",
				item:           image,
			})
		}
	}
}

func checkForUnpinnedFiles(validator *Validator, node *composer.Node) {
	for j, file := range node.Files {
		fileYqPath := fmt.Sprintf(".components.[%d].files.[%d]", node.Index(), j)
		if file.Shasum == "" && helpers.IsURL(file.Source) {
			validator.addWarning(validatorMessage{
				yqPath:         fileYqPath,
				packageRelPath: node.ImportLocation(),
				packageName:    node.OriginalPackageName(),
				description:    "No shasum for remote file",
				item:           file.Source,
			})
		}
	}
}

func checkForVarInComponentImport(validator *Validator, node *composer.Node) {
	if strings.Contains(node.Import.Path, types.ZarfPackageTemplatePrefix) {
		validator.addWarning(validatorMessage{
			yqPath:         fmt.Sprintf(".components.[%d].import.path", node.Index()),
			packageRelPath: node.ImportLocation(),
			packageName:    node.OriginalPackageName(),
			description:    "Zarf does not evaluate variables at component.x.import.path",
			item:           node.Import.Path,
		})
	}
	if strings.Contains(node.Import.URL, types.ZarfPackageTemplatePrefix) {
		validator.addWarning(validatorMessage{
			yqPath:         fmt.Sprintf(".components.[%d].import.url", node.Index()),
			packageRelPath: node.ImportLocation(),
			packageName:    node.OriginalPackageName(),
			description:    "Zarf does not evaluate variables at component.x.import.url",
			item:           node.Import.URL,
		})
	}
}

func makeFieldPathYqCompat(field string) string {
	if field == "(root)" {
		return field
	}
	// \b is a metacharacter that will stop at the next non-word character (including .)
	// https://regex101.com/r/pIRPk0/1
	re := regexp.MustCompile(`(\b\d+\b)`)

	wrappedField := re.ReplaceAllString(field, "[$1]")

	return fmt.Sprintf(".%s", wrappedField)
}

func validateSchema(validator *Validator, jsonSchema []byte, untypedZarfPackage map[string]interface{}) error {

	schemaErrors, err := runSchema(jsonSchema, untypedZarfPackage)
	if err != nil {
		return err
	}

	if len(schemaErrors) != 0 {
		for _, schemaErr := range schemaErrors {
			validator.addError(validatorMessage{
				yqPath:         makeFieldPathYqCompat(schemaErr.Field()),
				description:    schemaErr.Description(),
				packageRelPath: ".",
				packageName:    validator.zarfPackage.Metadata.Name,
			})
		}
	}

	return err
}

func runSchema(jsonSchema []byte, pkg interface{}) ([]gojsonschema.ResultError, error) {
	schemaLoader := gojsonschema.NewBytesLoader(jsonSchema)
	documentLoader := gojsonschema.NewGoLoader(pkg)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return nil, err
	}

	if !result.Valid() {
		return result.Errors(), nil
	}
	return nil, nil
}

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
	"github.com/defenseunicorns/zarf/src/pkg/packager/creator"
	"github.com/defenseunicorns/zarf/src/pkg/transform"
	"github.com/defenseunicorns/zarf/src/pkg/utils"
	"github.com/defenseunicorns/zarf/src/types"
	"github.com/xeipuuv/gojsonschema"
)

// FileLoader is an interface for reading files, it decouples the lint package from the go embed package which enables testing
type FileLoader interface {
	ReadFile(path string) ([]byte, error)
}

// ZarfSchema is exported so main.go can embed the schema file
var ZarfSchema FileLoader

// Validate validates a zarf file
func Validate(ctx context.Context, createOpts types.ZarfCreateOptions) ([]types.PackageFinding, error) {
	var pkg types.ZarfPackage
	var pkgErrs []types.PackageFinding
	if err := utils.ReadYaml(layout.ZarfYAML, &pkg); err != nil {
		return nil, err
	}
	compFindings, err := lintComponents(ctx, pkg, createOpts)
	if err != nil {
		return nil, err
	}
	pkgErrs = append(pkgErrs, compFindings...)

	jsonSchema, err := ZarfSchema.ReadFile("zarf.schema.json")
	if err != nil {
		return nil, err
	}

	var untypedZarfPackage interface{}
	if err := utils.ReadYaml(layout.ZarfYAML, &untypedZarfPackage); err != nil {
		return nil, err
	}

	schemaFindings, err := validateSchema(jsonSchema, untypedZarfPackage)
	if err != nil {
		return nil, err
	}
	pkgErrs = append(pkgErrs, schemaFindings...)

	return pkgErrs, nil
}

func lintComponents(ctx context.Context, pkg types.ZarfPackage, createOpts types.ZarfCreateOptions) ([]types.PackageFinding, error) {
	var pkgErrs []types.PackageFinding

	for i, component := range pkg.Components {
		arch := config.GetArch(pkg.Metadata.Architecture)
		if !composer.CompatibleComponent(component, arch, createOpts.Flavor) {
			continue
		}

		chain, err := composer.NewImportChain(ctx, component, i, pkg.Metadata.Name, arch, createOpts.Flavor)

		if err != nil {
			return nil, err
		}

		node := chain.Head()
		for node != nil {
			component := node.ZarfComponent
			nodeErrs := fillComponentTemplate(&component, &createOpts)
			nodeErrs = append(nodeErrs, checkComponent(component, node.Index())...)
			for i := range nodeErrs {
				nodeErrs[i].PackagePathOverride = node.ImportLocation()
				nodeErrs[i].PackageNameOverride = node.OriginalPackageName()
			}
			pkgErrs = append(pkgErrs, nodeErrs...)
			node = node.Next()
		}
	}
	return pkgErrs, nil
}

func fillComponentTemplate(c *types.ZarfComponent, createOpts *types.ZarfCreateOptions) []types.PackageFinding {
	err := creator.ReloadComponentTemplate(c)
	var nodeErrs []types.PackageFinding
	if err != nil {
		nodeErrs = append(nodeErrs, types.PackageFinding{
			Description: err.Error(),
			Category:    types.SevWarn,
		})
	}
	templateMap := map[string]string{}

	setVarsAndWarn := func(templatePrefix string, deprecated bool) {
		yamlTemplates, err := utils.FindYamlTemplates(c, templatePrefix, "###")
		if err != nil {
			nodeErrs = append(nodeErrs, types.PackageFinding{
				Description: err.Error(),
				Category:    types.SevWarn,
			})
		}

		for key := range yamlTemplates {
			if deprecated {
				nodeErrs = append(nodeErrs, types.PackageFinding{
					Description: fmt.Sprintf(lang.PkgValidateTemplateDeprecation, key, key, key),
					Category:    types.SevWarn,
				})
			}
			_, present := createOpts.SetVariables[key]
			if !present {
				nodeErrs = append(nodeErrs, types.PackageFinding{
					Description: lang.UnsetVarLintWarning,
					Category:    types.SevWarn,
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

	//nolint: errcheck // This error should bubble up
	utils.ReloadYamlTemplate(c, templateMap)
	return nodeErrs
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

// checkComponent runs lint rules against a component
func checkComponent(c types.ZarfComponent, i int) []types.PackageFinding {
	var pkgErrs []types.PackageFinding
	pkgErrs = append(pkgErrs, checkForUnpinnedRepos(c, i)...)
	pkgErrs = append(pkgErrs, checkForUnpinnedImages(c, i)...)
	pkgErrs = append(pkgErrs, checkForUnpinnedFiles(c, i)...)
	return pkgErrs
}

func checkForUnpinnedRepos(c types.ZarfComponent, i int) []types.PackageFinding {
	var pkgErrs []types.PackageFinding
	for j, repo := range c.Repos {
		repoYqPath := fmt.Sprintf(".components.[%d].repos.[%d]", i, j)
		if !isPinnedRepo(repo) {
			pkgErrs = append(pkgErrs, types.PackageFinding{
				YqPath:      repoYqPath,
				Description: "Unpinned repository",
				Item:        repo,
				Category:    types.SevWarn,
			})
		}
	}
	return pkgErrs
}

func checkForUnpinnedImages(c types.ZarfComponent, i int) []types.PackageFinding {
	var pkgErrs []types.PackageFinding
	for j, image := range c.Images {
		imageYqPath := fmt.Sprintf(".components.[%d].images.[%d]", i, j)
		pinnedImage, err := isPinnedImage(image)
		if err != nil {
			pkgErrs = append(pkgErrs, types.PackageFinding{
				YqPath:      imageYqPath,
				Description: "Failed to parse image reference",
				Item:        image,
				Category:    types.SevWarn,
			})
			continue
		}
		if !pinnedImage {
			pkgErrs = append(pkgErrs, types.PackageFinding{
				YqPath:      imageYqPath,
				Description: "Image not pinned with digest",
				Item:        image,
				Category:    types.SevWarn,
			})
		}
	}
	return pkgErrs
}

func checkForUnpinnedFiles(c types.ZarfComponent, i int) []types.PackageFinding {
	var pkgErrs []types.PackageFinding
	for j, file := range c.Files {
		fileYqPath := fmt.Sprintf(".components.[%d].files.[%d]", i, j)
		if file.Shasum == "" && helpers.IsURL(file.Source) {
			pkgErrs = append(pkgErrs, types.PackageFinding{
				YqPath:      fileYqPath,
				Description: "No shasum for remote file",
				Item:        file.Source,
				Category:    types.SevWarn,
			})
		}
	}
	return pkgErrs
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

func validateSchema(jsonSchema []byte, untypedZarfPackage interface{}) ([]types.PackageFinding, error) {
	var pkgErrs []types.PackageFinding

	schemaErrors, err := runSchema(jsonSchema, untypedZarfPackage)
	if err != nil {
		return nil, err
	}

	if len(schemaErrors) != 0 {
		for _, schemaErr := range schemaErrors {
			pkgErrs = append(pkgErrs, types.PackageFinding{
				YqPath:      makeFieldPathYqCompat(schemaErr.Field()),
				Description: schemaErr.Description(),
				Category:    types.SevErr,
			})
		}
	}

	return pkgErrs, err
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

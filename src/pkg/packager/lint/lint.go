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

// Validate validates a zarf file
func Validate(ctx context.Context, createOpts types.ZarfCreateOptions) ([]types.PackageError, error) {
	var pkg types.ZarfPackage
	var pkgErrs []types.PackageError
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

func lintComponents(ctx context.Context, pkg types.ZarfPackage, createOpts types.ZarfCreateOptions) ([]types.PackageError, error) {
	var pkgErrs []types.PackageError
	templateMap := map[string]string{}
	for key, value := range createOpts.SetVariables {
		templateMap[fmt.Sprintf("%s%s###", types.ZarfPackageTemplatePrefix, key)] = value
	}

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
			nodeErrs := checkForVarInComponentImport(component, node.Index())
			if err := utils.ReloadYamlTemplate(&component, templateMap); err != nil {
				return nil, err
			}
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
func checkComponent(c types.ZarfComponent, i int) []types.PackageError {
	var pkgErrs []types.PackageError
	pkgErrs = append(pkgErrs, checkForUnpinnedRepos(c, i)...)
	pkgErrs = append(pkgErrs, checkForUnpinnedImages(c, i)...)
	pkgErrs = append(pkgErrs, checkForUnpinnedFiles(c, i)...)
	return pkgErrs
}

func checkForUnpinnedRepos(c types.ZarfComponent, i int) []types.PackageError {
	var pkgErrs []types.PackageError
	for j, repo := range c.Repos {
		repoYqPath := fmt.Sprintf(".components.[%d].repos.[%d]", i, j)
		if !isPinnedRepo(repo) {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:      repoYqPath,
				Description: "Unpinned repository",
				Item:        repo,
				Category:    types.SevWarn,
			})
		}
	}
	return pkgErrs
}

func checkForUnpinnedImages(c types.ZarfComponent, i int) []types.PackageError {
	var pkgErrs []types.PackageError
	for j, image := range c.Images {
		imageYqPath := fmt.Sprintf(".components.[%d].images.[%d]", i, j)
		pinnedImage, err := isPinnedImage(image)
		if err != nil {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:      imageYqPath,
				Description: "Failed to parse image reference",
				Item:        image,
				Category:    types.SevWarn,
			})
			continue
		}
		if !pinnedImage {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:      imageYqPath,
				Description: "Image not pinned with digest",
				Item:        image,
				Category:    types.SevWarn,
			})
		}
	}
	return pkgErrs
}

func checkForUnpinnedFiles(c types.ZarfComponent, i int) []types.PackageError {
	var pkgErrs []types.PackageError
	for j, file := range c.Files {
		fileYqPath := fmt.Sprintf(".components.[%d].files.[%d]", i, j)
		if file.Shasum == "" && helpers.IsURL(file.Source) {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:      fileYqPath,
				Description: "No shasum for remote file",
				Item:        file.Source,
				Category:    types.SevWarn,
			})
		}
	}
	return pkgErrs
}

// TODO, this should be moved into the schema or we should add functionality so this is allowed
func checkForVarInComponentImport(c types.ZarfComponent, i int) []types.PackageError {
	var pkgErrs []types.PackageError
	if strings.Contains(c.Import.Path, types.ZarfPackageTemplatePrefix) {
		pkgErrs = append(pkgErrs, types.PackageError{
			YqPath:      fmt.Sprintf(".components.[%d].import.path", i),
			Description: "Zarf does not evaluate variables at component.x.import.path",
			Item:        c.Import.Path,
			Category:    types.SevWarn,
		})
	}
	if strings.Contains(c.Import.URL, types.ZarfPackageTemplatePrefix) {
		pkgErrs = append(pkgErrs, types.PackageError{
			YqPath:      fmt.Sprintf(".components.[%d].import.url", i),
			Description: "Zarf does not evaluate variables at component.x.import.url",
			Item:        c.Import.URL,
			Category:    types.SevWarn,
		})
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

func validateSchema(jsonSchema []byte, untypedZarfPackage interface{}) ([]types.PackageError, error) {
	var pkgErrs []types.PackageError

	schemaErrors, err := runSchema(jsonSchema, untypedZarfPackage)
	if err != nil {
		return nil, err
	}

	if len(schemaErrors) != 0 {
		for _, schemaErr := range schemaErrors {
			pkgErrs = append(pkgErrs, types.PackageError{
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

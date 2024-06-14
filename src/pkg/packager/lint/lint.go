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

// Validate validates a zarf file against the zarf schema, returns *validator with warnings or errors if they exist
// along with an error if the validation itself failed
func Validate(ctx context.Context, createOpts types.ZarfCreateOptions) ([]types.PackageError, error) {
	var pkg types.ZarfPackage
	var pkgErrs []types.PackageError
	if err := utils.ReadYaml(layout.ZarfYAML, pkg); err != nil {
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
	for i, component := range pkg.Components {
		arch := config.GetArch(pkg.Metadata.Architecture)
		if !composer.CompatibleComponent(component, arch, createOpts.Flavor) {
			continue
		}

		chain, err := composer.NewImportChain(ctx, component, i, pkg.Metadata.Name, arch, createOpts.Flavor)
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
			pkgErrs = append(pkgErrs, types.PackageError{
				Description: err.Error(),
				YqPath:      badImportYqPath,
				Category:    types.SevErr,
			})
		}

		node := baseComponent
		for node != nil {
			pkgErrs = append(pkgErrs, checkForVarInComponentImport(node)...)
			pkgErrs = append(pkgErrs, lintComponent(node)...)
			node = node.Next()
		}
	}
	return pkgErrs, nil
}

func isPinnedImage(image string) (bool, error) {
	transformedImage, err := transform.ParseImageRef(image)
	if err != nil {
		if strings.Contains(image, types.ZarfPackageTemplatePrefix) ||
			//TODO check if it's even reasonable to use a variable here
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

func lintComponent(node *composer.Node) []types.PackageError {
	var pkgErrs []types.PackageError
	pkgErrs = append(pkgErrs, checkForUnpinnedRepos(node)...)
	pkgErrs = append(pkgErrs, checkForUnpinnedImages(node)...)
	pkgErrs = append(pkgErrs, checkForUnpinnedFiles(node)...)
	return pkgErrs
}

func checkForUnpinnedRepos(node *composer.Node) []types.PackageError {
	var pkgErrs []types.PackageError
	for j, repo := range node.Repos {
		repoYqPath := fmt.Sprintf(".components.[%d].repos.[%d]", node.Index(), j)
		if !isPinnedRepo(repo) {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:              repoYqPath,
				PackagePathOverride: node.ImportLocation(),
				PackageNameOverride: node.OriginalPackageName(),
				Description:         "Unpinned repository",
				Item:                repo,
				Category:            types.SevWarn,
			})
		}
	}
	return pkgErrs
}

func checkForUnpinnedImages(node *composer.Node) []types.PackageError {
	var pkgErrs []types.PackageError
	for j, image := range node.Images {
		imageYqPath := fmt.Sprintf(".components.[%d].images.[%d]", node.Index(), j)
		pinnedImage, err := isPinnedImage(image)
		if err != nil {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:              imageYqPath,
				PackagePathOverride: node.ImportLocation(),
				PackageNameOverride: node.OriginalPackageName(),
				Description:         "Invalid image reference",
				Item:                image,
				Category:            types.SevErr,
			})
			continue
		}
		if !pinnedImage {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:              imageYqPath,
				PackagePathOverride: node.ImportLocation(),
				PackageNameOverride: node.OriginalPackageName(),
				Description:         "Image not pinned with digest",
				Item:                image,
				Category:            types.SevWarn,
			})
		}
	}
	return pkgErrs
}

func checkForUnpinnedFiles(node *composer.Node) []types.PackageError {
	var pkgErrs []types.PackageError
	for j, file := range node.Files {
		fileYqPath := fmt.Sprintf(".components.[%d].files.[%d]", node.Index(), j)
		if file.Shasum == "" && helpers.IsURL(file.Source) {
			pkgErrs = append(pkgErrs, types.PackageError{
				YqPath:              fileYqPath,
				PackagePathOverride: node.ImportLocation(),
				PackageNameOverride: node.OriginalPackageName(),
				Description:         "No shasum for remote file",
				Item:                file.Source,
			})
		}
	}
	return pkgErrs
}

func checkForVarInComponentImport(node *composer.Node) []types.PackageError {
	var pkgErrs []types.PackageError
	if strings.Contains(node.Import.Path, types.ZarfPackageTemplatePrefix) {
		pkgErrs = append(pkgErrs, types.PackageError{
			YqPath:              fmt.Sprintf(".components.[%d].import.path", node.Index()),
			PackagePathOverride: node.ImportLocation(),
			PackageNameOverride: node.OriginalPackageName(),
			Description:         "Zarf does not evaluate variables at component.x.import.path",
			Item:                node.Import.Path,
		})
	}
	if strings.Contains(node.Import.URL, types.ZarfPackageTemplatePrefix) {
		pkgErrs = append(pkgErrs, types.PackageError{
			YqPath:              fmt.Sprintf(".components.[%d].import.url", node.Index()),
			PackagePathOverride: node.ImportLocation(),
			PackageNameOverride: node.OriginalPackageName(),
			Description:         "Zarf does not evaluate variables at component.x.import.url",
			Item:                node.Import.URL,
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

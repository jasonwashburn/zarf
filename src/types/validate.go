// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The Zarf Authors

// Package types contains all the types used by Zarf.
package types

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/defenseunicorns/zarf/src/config/lang"
)

const (
	// ZarfMaxChartNameLength limits helm chart name size to account for K8s/helm limits and zarf prefix
	ZarfMaxChartNameLength = 40
)

var (
	// IsLowercaseNumberHyphenNoStartHyphen is a regex for lowercase, numbers and hyphens that cannot start with a hyphen.
	// https://regex101.com/r/FLdG9G/2
	IsLowercaseNumberHyphenNoStartHyphen = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]*$`).MatchString
	// Define allowed OS, an empty string means it is allowed on all operating systems
	// same as enums on ZarfComponentOnlyTarget
	supportedOS = []string{"linux", "darwin", "windows", ""}
)

// SupportedOS returns the supported operating systems.
//
// The supported operating systems are: linux, darwin, windows.
//
// An empty string signifies no OS restrictions.
func SupportedOS() []string {
	return supportedOS
}

// Validate runs all validation checks on the package.
func (pkg ZarfPackage) Validate() error {
	var err error
	if pkg.Kind == ZarfInitConfig && pkg.Metadata.YOLO {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrInitNoYOLO))
	}

	if !IsLowercaseNumberHyphenNoStartHyphen(pkg.Metadata.Name) {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrPkgName, pkg.Metadata.Name))
	}

	if len(pkg.Components) == 0 {
		err = errors.Join(err, fmt.Errorf("package must have at least 1 component"))
	}

	for _, variable := range pkg.Variables {
		if varErr := variable.Validate(); varErr != nil {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrVariable, varErr))
		}
	}

	for _, constant := range pkg.Constants {
		if varErr := constant.Validate(); varErr != nil {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrConstant, varErr))
		}
	}

	uniqueComponentNames := make(map[string]bool)
	groupDefault := make(map[string]string)
	groupedComponents := make(map[string][]string)

	if pkg.Metadata.YOLO {
		for _, component := range pkg.Components {
			if len(component.Images) > 0 {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrYOLONoOCI))
			}

			if len(component.Repos) > 0 {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrYOLONoGit))
			}

			if component.Only.Cluster.Architecture != "" {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrYOLONoArch))
			}

			if len(component.Only.Cluster.Distros) > 0 {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrYOLONoDistro))
			}
		}
	}

	for _, component := range pkg.Components {
		// ensure component name is unique
		if _, ok := uniqueComponentNames[component.Name]; ok {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrComponentNameNotUnique, component.Name))
		}
		uniqueComponentNames[component.Name] = true

		if !IsLowercaseNumberHyphenNoStartHyphen(component.Name) {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrComponentName, component.Name))
		}

		if !slices.Contains(supportedOS, component.Only.LocalOS) {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrComponentLocalOS, component.Name, component.Only.LocalOS, supportedOS))
		}

		if component.IsRequired() {
			if component.Default {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrComponentReqDefault, component.Name))
			}
			if component.DeprecatedGroup != "" {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrComponentReqGrouped, component.Name))
			}
		}

		uniqueChartNames := make(map[string]bool)
		for _, chart := range component.Charts {
			// ensure chart name is unique
			if _, ok := uniqueChartNames[chart.Name]; ok {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChartNameNotUnique, chart.Name))
			}
			uniqueChartNames[chart.Name] = true

			if chartErr := chart.Validate(); chartErr != nil {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChart, chartErr))
			}
		}

		uniqueManifestNames := make(map[string]bool)
		for _, manifest := range component.Manifests {
			// ensure manifest name is unique
			if _, ok := uniqueManifestNames[manifest.Name]; ok {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrManifestNameNotUnique, manifest.Name))
			}
			uniqueManifestNames[manifest.Name] = true

			if manifestErr := manifest.Validate(); manifestErr != nil {
				err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrManifest, manifestErr))
			}
		}

		// ensure groups don't have multiple defaults or only one component
		if component.DeprecatedGroup != "" {
			if component.Default {
				if _, ok := groupDefault[component.DeprecatedGroup]; ok {
					err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrGroupMultipleDefaults, component.DeprecatedGroup, groupDefault[component.DeprecatedGroup], component.Name))
				}
				groupDefault[component.DeprecatedGroup] = component.Name
			}
			groupedComponents[component.DeprecatedGroup] = append(groupedComponents[component.DeprecatedGroup], component.Name)
		}
	}

	for groupKey, componentNames := range groupedComponents {
		if len(componentNames) == 1 {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrGroupOneComponent, groupKey, componentNames[0]))
		}
	}

	return err
}

// Validate validates the component trying to be imported.
func (c ZarfComponent) Validate() error {
	var err error
	path := c.Import.Path
	url := c.Import.URL

	// ensure path or url is provided
	if path == "" && url == "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrImportDefinition, c.Name, "neither a path nor a URL was provided"))
	}

	// ensure path and url are not both provided
	if path != "" && url != "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrImportDefinition, c.Name, "both a path and a URL were provided"))
	}

	// validation for path
	if url == "" && path != "" {
		// ensure path is not an absolute path
		if filepath.IsAbs(path) {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrImportDefinition, c.Name, "path cannot be an absolute path"))
		}
	}

	// validation for url
	if url != "" && path == "" {
		ok := helpers.IsOCIURL(url)
		if !ok {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrImportDefinition, c.Name, "URL is not a valid OCI URL"))
		}
	}

	return err
}

// Validate runs all validation checks on an action.
func (action ZarfComponentAction) Validate() error {
	var err error
	for _, variable := range action.SetVariables {
		err = errors.Join(err, variable.Validate())
	}

	if action.Wait != nil {
		// Validate only cmd or wait, not both
		if action.Cmd != "" {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrActionCmdWait, action.Cmd))
		}

		// Validate only cluster or network, not both
		if action.Wait.Cluster != nil && action.Wait.Network != nil {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrActionClusterNetwork))
		}

		// Validate at least one of cluster or network
		if action.Wait.Cluster == nil && action.Wait.Network == nil {
			err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrActionClusterNetwork))
		}
	}

	return err
}

// Validate runs all validation checks on a chart.
func (chart ZarfChart) Validate() error {
	var err error

	if chart.Name == "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChartNameMissing))
	}

	if len(chart.Name) > ZarfMaxChartNameLength {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChartName, chart.Name, ZarfMaxChartNameLength))
	}

	if chart.Namespace == "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChartNamespaceMissing, chart.Name))
	}

	// Must have a url or localPath (and not both)
	if chart.URL != "" && chart.LocalPath != "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChartURLOrPath, chart.Name))
	}

	if chart.URL == "" && chart.LocalPath == "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChartURLOrPath, chart.Name))
	}

	if chart.Version == "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrChartVersion, chart.Name))
	}

	return err
}

// Validate runs all validation checks on a manifest.
func (manifest ZarfManifest) Validate() error {
	var err error

	if manifest.Name == "" {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrManifestNameMissing))
	}

	if len(manifest.Name) > ZarfMaxChartNameLength {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrManifestNameLength, manifest.Name, ZarfMaxChartNameLength))
	}

	if len(manifest.Files) < 1 && len(manifest.Kustomizations) < 1 {
		err = errors.Join(err, fmt.Errorf(lang.PkgValidateErrManifestFileOrKustomize, manifest.Name))
	}

	return err
}

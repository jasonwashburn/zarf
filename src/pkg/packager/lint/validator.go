// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2021-Present The Zarf Authors

// Package lint contains functions for verifying zarf yaml files are valid
package lint

import (
	"fmt"
	"path/filepath"

	"github.com/defenseunicorns/pkg/helpers"
	"github.com/defenseunicorns/zarf/src/pkg/message"
	"github.com/defenseunicorns/zarf/src/types"
	"github.com/fatih/color"
)

type Category int

const (
	CategoryError   Category = 1
	CategoryWarning Category = 2
)

type validatorMessage struct {
	yqPath         string
	description    string
	item           string
	packageRelPath string
	packageName    string
	category       Category
}

func (c Category) String() string {
	if c == CategoryError {
		return message.ColorWrap("Error", color.FgRed)
	} else if c == CategoryWarning {
		return message.ColorWrap("Warning", color.FgYellow)
	}
	return ""
}

func (vm validatorMessage) String() string {
	if vm.item != "" {
		vm.item = fmt.Sprintf(" - %s", vm.item)
	}
	return fmt.Sprintf("%s%s", vm.description, vm.item)
}

// Validator holds the warnings/errors and messaging that we get from validation
type Validator struct {
	findings           []validatorMessage
	typedZarfPackage   types.ZarfPackage
	untypedZarfPackage interface{}
	baseDir            string
}

// DisplayFormattedMessage message sent to user based on validator results
func (v Validator) DisplayFormattedMessage() {
	if !v.hasFindings() {
		message.Successf("0 findings for %q", v.typedZarfPackage.Metadata.Name)
	}
	v.PrintValidationTable(CategoryWarning)
}

// IsSuccess returns true if there are not any errors
func (v Validator) IsSuccess() bool {
	for _, finding := range v.findings {
		if finding.category == CategoryError {
			return false
		}
	}
	return true
}

func (v Validator) packageRelPathToUser(vm validatorMessage) string {
	if helpers.IsOCIURL(vm.packageRelPath) {
		return vm.packageRelPath
	}
	return filepath.Join(v.baseDir, vm.packageRelPath)
}

func (v Validator) PrintValidationTable(severity Category) {
	if !v.hasSeverity(severity) {
		return
	}

	mapOfFindingsByPath := make(map[string][]validatorMessage)
	for _, finding := range v.findings {
		if finding.category <= severity {
			mapOfFindingsByPath[finding.packageRelPath] = append(mapOfFindingsByPath[finding.packageRelPath], finding)
		}
	}

	header := []string{"Type", "Path", "Message"}

	for packageRelPath, findings := range mapOfFindingsByPath {
		lintData := [][]string{}
		for _, finding := range findings {
			if finding.category <= severity {
				lintData = append(lintData, []string{finding.category.String(), finding.getPath(), finding.String()})
			}
		}
		message.Notef("Linting package %q at %s", findings[0].packageName, v.packageRelPathToUser(findings[0]))
		message.Table(header, lintData)
		message.Info(v.getFormattedFindingCount(packageRelPath, findings[0].packageName))
	}
}

func (v Validator) getFormattedFindingCount(relPath string, packageName string) string {
	warningCount := 0
	errorCount := 0
	for _, finding := range v.findings {
		if finding.packageRelPath != relPath {
			continue
		}
		if finding.category == CategoryWarning {
			warningCount++
		}
		if finding.category == CategoryError {
			errorCount++
		}
	}
	wordWarning := "warnings"
	if warningCount == 1 {
		wordWarning = "warning"
	}
	wordError := "errors"
	if errorCount == 1 {
		wordError = "error"
	}
	return fmt.Sprintf("%d %s and %d %s in %q",
		warningCount, wordWarning, errorCount, wordError, packageName)
}

func (vm validatorMessage) getPath() string {
	if vm.yqPath == "" {
		return ""
	}
	return message.ColorWrap(vm.yqPath, color.FgCyan)
}

func (v Validator) hasFindings() bool {
	return len(v.findings) > 0
}

func (v Validator) hasSeverity(category Category) bool {
	for _, finding := range v.findings {
		if finding.category <= category {
			return true
		}
	}
	return false
}

func (v Validator) HasErrors() bool {
	return v.hasSeverity(CategoryError)
}

func (v *Validator) addWarning(vmessage validatorMessage) {
	vmessage.category = CategoryWarning
	v.findings = helpers.Unique(append(v.findings, vmessage))
}

func (v *Validator) addError(vMessage validatorMessage) {
	vMessage.category = CategoryError
	v.findings = helpers.Unique(append(v.findings, vMessage))
}

package action

import (
	"fmt"
	"strings"

	cyclonedx "github.com/CycloneDX/cyclonedx-go"
	"github.com/jfrog/jfrog-cli-security/sca/bom/xrayplugin"
	"github.com/jfrog/jfrog-cli-security/utils/results"
	"github.com/jfrog/jfrog-cli-security/utils/techutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// FindDescriptorPaths runs the Xray-Lib plugin locally to build a dependency tree,
// then returns all descriptor file paths that contain componentName at affectedVersion.
// The component may appear in multiple files (e.g. multi-module projects).
func FindDescriptorPaths(workspaceDir, componentName, affectedVersion string) ([]string, error) {
	log.Debug("Preparing Xray-Lib plugin for dependency tree analysis")
	generator := xrayplugin.NewXrayLibBomGenerator()
	if err := generator.PrepareGenerator(); err != nil {
		return nil, fmt.Errorf("failed to prepare Xray-Lib plugin: %w", err)
	}

	log.Debug(fmt.Sprintf("Generating SBOM for workspace: %s", workspaceDir))
	sbom, err := generator.GenerateSbom(results.ScanTarget{Target: workspaceDir})
	if err != nil {
		return nil, fmt.Errorf("failed to generate SBOM: %w", err)
	}
	if sbom.Components != nil {
		log.Debug(fmt.Sprintf("SBOM generated with %d components", len(*sbom.Components)))
	}

	log.Debug(fmt.Sprintf("Searching SBOM for '%s@%s'", componentName, affectedVersion))
	paths, err := extractDescriptorPaths(sbom, componentName, affectedVersion)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("component '%s@%s' not found in SBOM", componentName, affectedVersion)
	}
	log.Info(fmt.Sprintf("Found '%s@%s' in %d descriptor file(s): %v", componentName, affectedVersion, len(paths), paths))
	return paths, nil
}

// extractDescriptorPaths walks the SBOM and collects every Evidence.Occurrence location
// for components whose name+version match the target.
func extractDescriptorPaths(sbom *cyclonedx.BOM, componentName, affectedVersion string) ([]string, error) {
	if sbom == nil || sbom.Components == nil {
		return nil, fmt.Errorf("SBOM is empty")
	}

	// Normalise component names for comparison: Maven "groupId:artifactId" is stored
	// as "groupId/artifactId" inside a PURL, so we convert both sides to slash-separated.
	normalise := func(name string) string {
		return strings.ReplaceAll(name, ":", "/")
	}
	wantName := normalise(componentName)

	seen := map[string]bool{}
	var paths []string

	for _, component := range *sbom.Components {
		compName, compVersion, compType := techutils.SplitPackageURL(component.PackageURL)
		log.Debug(fmt.Sprintf("Inspecting SBOM component: %s@%s (type: %s)", compName, compVersion, compType))

		if normalise(compName) != wantName || compVersion != affectedVersion {
			continue
		}
		log.Debug(fmt.Sprintf("Matched component '%s@%s' — checking evidence occurrences", compName, compVersion))

		if component.Evidence == nil || component.Evidence.Occurrences == nil {
			log.Debug(fmt.Sprintf("Component '%s@%s' has no evidence occurrences, skipping", compName, compVersion))
			continue
		}
		for _, occurrence := range *component.Evidence.Occurrences {
			if occurrence.Location == "" {
				continue
			}
			if seen[occurrence.Location] {
				log.Debug(fmt.Sprintf("Skipping duplicate occurrence location: %s", occurrence.Location))
				continue
			}
			seen[occurrence.Location] = true
			paths = append(paths, occurrence.Location)
			log.Debug(fmt.Sprintf("Found descriptor: %s", occurrence.Location))
		}
	}
	return paths, nil
}

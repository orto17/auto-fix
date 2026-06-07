package packageupdaters

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	mavenDependencySeparator = ":"
	propertyPrefix           = "${"
	propertySuffix           = "}"
	pomFileName              = "pom.xml"
)

type MavenPackageUpdater struct{}

type mavenProject struct {
	XMLName              xml.Name            `xml:"project"`
	Parent               *mavenDep           `xml:"parent"`
	Properties           *mavenProperties    `xml:"properties"`
	Dependencies         []mavenDep          `xml:"dependencies>dependency"`
	DependencyManagement *mavenDepManagement `xml:"dependencyManagement"`
}

type mavenProperties struct {
	Props []mavenProperty `xml:",any"`
}

type mavenProperty struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type mavenDep struct {
	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type mavenDepManagement struct {
	Dependencies []mavenDep `xml:"dependencies>dependency"`
}

func (m *MavenPackageUpdater) UpdateDependency(details *FixDetails) error {
	groupId, artifactId, err := parseMavenComponentName(details.ComponentName)
	if err != nil {
		return err
	}

	var errs error
	var failingDescriptors []string
	for _, pomPath := range details.DescriptorPaths {
		if fixErr := m.updatePomFile(pomPath, groupId, artifactId, details.FixVersion); fixErr != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to fix '%s' in '%s': %w", details.ComponentName, pomPath, fixErr))
			failingDescriptors = append(failingDescriptors, pomPath)
		}
	}

	if errs != nil {
		return fmt.Errorf("errors updating '%s' in [%s]: %w", details.ComponentName, strings.Join(failingDescriptors, ", "), errs)
	}
	return nil
}

func (m *MavenPackageUpdater) updatePomFile(pomPath, groupId, artifactId, fixedVersion string) error {
	content, err := os.ReadFile(pomPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", pomPath, err)
	}

	var project mavenProject
	if err = xml.Unmarshal(content, &project); err != nil {
		return fmt.Errorf("failed to parse %s: %w", pomPath, err)
	}

	current := content
	updatedAny := false

	if updated, c := m.updateInParent(&project, groupId, artifactId, fixedVersion, current); updated {
		current = c
		updatedAny = true
	}
	if updated, c := m.updateInDependencies(&project, project.Dependencies, groupId, artifactId, fixedVersion, current); updated {
		current = c
		updatedAny = true
	}
	if project.DependencyManagement != nil {
		if updated, c := m.updateInDependencies(&project, project.DependencyManagement.Dependencies, groupId, artifactId, fixedVersion, current); updated {
			current = c
			updatedAny = true
		}
	}

	if !updatedAny {
		return fmt.Errorf("dependency %s:%s not found in %s", groupId, artifactId, pomPath)
	}
	return os.WriteFile(pomPath, current, 0644)
}

func parseMavenComponentName(name string) (groupId, artifactId string, err error) {
	parts := strings.Split(name, mavenDependencySeparator)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid Maven component name '%s': expected 'groupId:artifactId'", name)
	}
	return parts[0], parts[1], nil
}

func (m *MavenPackageUpdater) updateInParent(project *mavenProject, groupId, artifactId, fixedVersion string, content []byte) (bool, []byte) {
	if project.Parent == nil || project.Parent.GroupId != groupId || project.Parent.ArtifactId != artifactId {
		return false, content
	}
	pattern := regexp.MustCompile(`(?s)(<parent>\s*<groupId>` + regexp.QuoteMeta(groupId) + `</groupId>\s*<artifactId>` + regexp.QuoteMeta(artifactId) + `</artifactId>\s*<version>)[^<]+(</version>)`)
	newContent := pattern.ReplaceAll(content, []byte("${1}"+fixedVersion+"${2}"))
	if bytes.Equal(content, newContent) {
		return false, content
	}
	return true, newContent
}

func (m *MavenPackageUpdater) updateInDependencies(project *mavenProject, deps []mavenDep, groupId, artifactId, fixedVersion string, content []byte) (bool, []byte) {
	for _, dep := range deps {
		if dep.GroupId != groupId || dep.ArtifactId != artifactId {
			continue
		}
		if propertyName, isProperty := extractPropertyName(dep.Version); isProperty {
			return m.updateProperty(project, propertyName, fixedVersion, content)
		}
		pattern := regexp.MustCompile(`(?s)(<groupId>` + regexp.QuoteMeta(groupId) + `</groupId>\s*<artifactId>` + regexp.QuoteMeta(artifactId) + `</artifactId>\s*<version>)[^<]+(</version>)`)
		newContent := pattern.ReplaceAll(content, []byte("${1}"+fixedVersion+"${2}"))
		if !bytes.Equal(content, newContent) {
			return true, newContent
		}
	}
	return false, content
}

func extractPropertyName(version string) (string, bool) {
	if strings.HasPrefix(version, propertyPrefix) && strings.HasSuffix(version, propertySuffix) {
		return strings.TrimSuffix(strings.TrimPrefix(version, propertyPrefix), propertySuffix), true
	}
	return "", false
}

func (m *MavenPackageUpdater) updateProperty(project *mavenProject, propertyName, newValue string, content []byte) (bool, []byte) {
	if project.Properties == nil {
		return false, content
	}
	for _, prop := range project.Properties.Props {
		if prop.XMLName.Local == propertyName {
			pattern := regexp.MustCompile(`(<` + regexp.QuoteMeta(propertyName) + `>)[^<]+(</` + regexp.QuoteMeta(propertyName) + `>)`)
			newContent := pattern.ReplaceAll(content, []byte("${1}"+newValue+"${2}"))
			if !bytes.Equal(content, newContent) {
				return true, newContent
			}
		}
	}
	return false, content
}

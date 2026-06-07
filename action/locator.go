package action

import (
	"fmt"
	"strings"

	"github.com/jfrog/auto-fix/action/packageupdaters"
)

type Technology string

const (
	TechnologyMaven Technology = "maven"
	TechnologyNPM   Technology = "npm"
)

// DetectTechnology infers the technology from the component name.
// Maven components always contain ":" (groupId:artifactId).
// NPM packages never do.
func DetectTechnology(componentName string) Technology {
	if strings.Contains(componentName, ":") {
		return TechnologyMaven
	}
	return TechnologyNPM
}

// LocateDescriptors finds descriptor files in the working directory for the given technology.
func LocateDescriptors(tech Technology) ([]string, error) {
	switch tech {
	case TechnologyMaven:
		paths, err := packageupdaters.GetAllDescriptorFilesFullPaths([]string{"pom.xml"})
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			return nil, fmt.Errorf("no pom.xml found in repository")
		}
		return paths, nil
	case TechnologyNPM:
		paths, err := packageupdaters.GetAllDescriptorFilesFullPaths([]string{"package.json"}, "node_modules")
		if err != nil {
			return nil, err
		}
		if len(paths) == 0 {
			return nil, fmt.Errorf("no package.json found in repository")
		}
		return paths, nil
	default:
		return nil, fmt.Errorf("unsupported technology: %s", tech)
	}
}

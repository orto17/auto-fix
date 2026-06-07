package packageupdaters

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	nodePackageJSONFileName          = "package.json"
	nodeModulesDirName               = "node_modules"
	nodeDependenciesSection          = "dependencies"
	nodeDevDependenciesSection       = "devDependencies"
	nodeOptionalDependenciesSection  = "optionalDependencies"
	nodeOverridesSection             = "overrides"
	nodePackageManagerInstallTimeout = 15 * time.Minute
)

var nodePackageManifestSections = []string{
	nodeDependenciesSection,
	nodeDevDependenciesSection,
	nodeOptionalDependenciesSection,
	nodeOverridesSection,
}

// PackageUpdater is the interface each technology updater implements.
type PackageUpdater interface {
	UpdateDependency(details *FixDetails) error
}

type commonPackageUpdater struct{}

func (c *commonPackageUpdater) escapeJsonPathKey(key string) string {
	r := strings.NewReplacer(".", "\\.", "*", "\\*", "?", "\\?")
	return r.Replace(key)
}

func (c *commonPackageUpdater) getFixedPackageJSONManifest(content []byte, packageName, newVersion, descriptorPath string) ([]byte, error) {
	updated := false
	escapedName := c.escapeJsonPathKey(packageName)

	for _, section := range nodePackageManifestSections {
		path := section + "." + escapedName
		if gjson.GetBytes(content, path).Exists() {
			var err error
			content, err = sjson.SetBytes(content, path, newVersion)
			if err != nil {
				return nil, fmt.Errorf("failed to set version for '%s' in section '%s': %w", packageName, section, err)
			}
			updated = true
		}
	}

	if !updated {
		return nil, fmt.Errorf("package '%s' not found in allowed sections [%s] in '%s'", packageName, strings.Join(nodePackageManifestSections, ", "), descriptorPath)
	}
	return content, nil
}

func (c *commonPackageUpdater) updatePackageJSONDescriptor(descriptorPath, packageName, newVersion string) (backupContent []byte, err error) {
	descriptorContent, err := os.ReadFile(descriptorPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file '%s': %w", descriptorPath, err)
	}

	backupContent = make([]byte, len(descriptorContent))
	copy(backupContent, descriptorContent)

	updatedContent, err := c.getFixedPackageJSONManifest(descriptorContent, packageName, newVersion, descriptorPath)
	if err != nil {
		return nil, err
	}

	if err = os.WriteFile(descriptorPath, updatedContent, 0644); err != nil {
		return nil, fmt.Errorf("failed to write updated descriptor '%s': %w", descriptorPath, err)
	}
	return backupContent, nil
}

func (c *commonPackageUpdater) withDescriptorWorkingDir(descriptorPath, originalWd string, fn func() error) (err error) {
	descriptorDir := filepath.Dir(descriptorPath)
	if err = os.Chdir(descriptorDir); err != nil {
		return fmt.Errorf("failed to change directory to '%s': %w", descriptorDir, err)
	}
	defer func() {
		if chErr := os.Chdir(originalWd); chErr != nil {
			err = fmt.Errorf("%w; failed to return to original directory: %v", err, chErr)
		}
	}()
	return fn()
}

func (c *commonPackageUpdater) buildEnvWithOverrides(overrides map[string]string) []string {
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, e := range os.Environ() {
		key := strings.SplitN(e, "=", 2)[0]
		if _, shouldOverride := overrides[key]; !shouldOverride {
			env = append(env, e)
		}
	}
	for key, value := range overrides {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}

// GetAllDescriptorFilesFullPaths walks the working directory and returns all
// files whose names match one of descriptorFileNames, skipping patternsToExclude.
func GetAllDescriptorFilesFullPaths(descriptorFileNames []string, patternsToExclude ...string) ([]string, error) {
	if len(descriptorFileNames) == 0 {
		return nil, nil
	}

	nameSet := make(map[string]bool, len(descriptorFileNames))
	for _, n := range descriptorFileNames {
		nameSet[n] = true
	}

	var excludeRe []*regexp.Regexp
	for _, p := range patternsToExclude {
		excludeRe = append(excludeRe, regexp.MustCompile(p))
	}

	var result []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, innerErr error) error {
		if innerErr != nil {
			return innerErr
		}
		for _, re := range excludeRe {
			if re.FindString(path) != "" {
				return filepath.SkipDir
			}
		}
		if nameSet[filepath.Base(path)] {
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			result = append(result, abs)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}
	return result, nil
}

package action

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jfrog/auto-fix/action/packageupdaters"
	"github.com/jfrog/jfrog-cli-security/utils/techutils"
	"github.com/jfrog/jfrog-client-go/utils/log"
)

// initLogger sets the global JFrog logger level from the INPUT_LOG_LEVEL env var.
// Accepted values (case-insensitive): DEBUG, INFO, WARN, ERROR. Defaults to INFO.
// We pass os.Stdout explicitly so every level (including DEBUG) appears in the
// same stream that GitHub Actions captures and displays in the run log.
func initLogger() {
	rawLevel := os.Getenv("INPUT_LOG_LEVEL")
	level := log.INFO
	switch strings.ToUpper(rawLevel) {
	case "DEBUG":
		level = log.DEBUG
	case "WARN":
		level = log.WARN
	case "ERROR":
		level = log.ERROR
	}
	log.SetLogger(log.NewLogger(level, os.Stdout))
}

type Inputs struct {
	ComponentName   string
	AffectedVersion string
	FixVersion      string
	GitHubToken     string
	RepoOwner       string
	RepoName        string
	BranchName      string
	CommitHash      string
	WorkspaceDir    string
	LogLevel        string
}

func ReadInputs() (Inputs, error) {
	initLogger()

	in := Inputs{
		ComponentName:   os.Getenv("INPUT_COMPONENT_NAME"),
		AffectedVersion: os.Getenv("INPUT_AFFECTED_VERSION"),
		FixVersion:      os.Getenv("INPUT_FIX_VERSION"),
		GitHubToken:     os.Getenv("INPUT_GITHUB_TOKEN"),
		BranchName:      os.Getenv("INPUT_BRANCH_NAME"),
		CommitHash:      os.Getenv("INPUT_COMMIT_HASH"),
		WorkspaceDir:    os.Getenv("GITHUB_WORKSPACE"),
		LogLevel:        os.Getenv("INPUT_LOG_LEVEL"),
	}

	repo := os.Getenv("GITHUB_REPOSITORY")
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		in.RepoOwner = parts[0]
		in.RepoName = parts[1]
	}

	if err := in.validate(); err != nil {
		return Inputs{}, err
	}
	return in, nil
}

func (in Inputs) validate() error {
	required := map[string]string{
		"component_name":   in.ComponentName,
		"affected_version": in.AffectedVersion,
		"fix_version":      in.FixVersion,
		"github_token":     in.GitHubToken,
		"branch_name":      in.BranchName,
		"commit_hash":      in.CommitHash,
	}
	for name, val := range required {
		if val == "" {
			return fmt.Errorf("missing required input: %s", name)
		}
	}
	if in.RepoOwner == "" || in.RepoName == "" {
		return fmt.Errorf("could not determine repository owner/name from GITHUB_REPOSITORY")
	}
	return nil
}

// Run is the main action logic.
func Run(ctx context.Context, in Inputs) error {
	log.Debug(fmt.Sprintf("Log level: %s", in.LogLevel))
	log.Info(fmt.Sprintf("Starting auto-fix for component '%s' (%s → %s) in %s/%s",
		in.ComponentName, in.AffectedVersion, in.FixVersion, in.RepoOwner, in.RepoName))

	if in.WorkspaceDir != "" {
		log.Debug(fmt.Sprintf("Changing working directory to workspace: %s", in.WorkspaceDir))
		if err := os.Chdir(in.WorkspaceDir); err != nil {
			return fmt.Errorf("failed to chdir to workspace: %w", err)
		}
	}

	log.Info(fmt.Sprintf("Scanning project to locate '%s@%s' in dependency tree...", in.ComponentName, in.AffectedVersion))
	descriptorPaths, err := FindDescriptorPaths(in.WorkspaceDir, in.ComponentName, in.AffectedVersion)
	if err != nil {
		return err
	}

	updater, err := newUpdater(in.ComponentName)
	if err != nil {
		return err
	}
	log.Debug(fmt.Sprintf("Selected updater for component '%s'", in.ComponentName))

	details := &packageupdaters.FixDetails{
		ComponentName:   in.ComponentName,
		AffectedVersion: in.AffectedVersion,
		FixVersion:      in.FixVersion,
		DescriptorPaths: descriptorPaths,
	}

	log.Info(fmt.Sprintf("Updating '%s' from %s to %s in %d file(s): %v",
		in.ComponentName, in.AffectedVersion, in.FixVersion, len(descriptorPaths), descriptorPaths))
	if err = updater.UpdateDependency(details); err != nil {
		return fmt.Errorf("failed to update dependency: %w", err)
	}
	log.Info(fmt.Sprintf("Successfully updated '%s' to %s", in.ComponentName, in.FixVersion))

	prURL, err := CreateFixPR(ctx, PRConfig{
		Token:           in.GitHubToken,
		Owner:           in.RepoOwner,
		Repo:            in.RepoName,
		BaseBranch:      in.BranchName,
		CommitHash:      in.CommitHash,
		ComponentName:   in.ComponentName,
		AffectedVersion: in.AffectedVersion,
		FixVersion:      in.FixVersion,
	})
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("Pull request ready: %s", prURL))
	_ = os.WriteFile(os.Getenv("GITHUB_OUTPUT"), []byte("pr_url="+prURL+"\n"), 0644)
	return nil
}

// newUpdater picks the right PackageUpdater based on the component name format.
// Maven uses "groupId:artifactId"; NPM packages never contain ":".
func newUpdater(componentName string) (packageupdaters.PackageUpdater, error) {
	tech := inferTechnology(componentName)
	log.Debug(fmt.Sprintf("Inferred technology '%s' from component name '%s'", tech, componentName))
	switch tech {
	case techutils.Maven:
		return &packageupdaters.MavenPackageUpdater{}, nil
	case techutils.Npm:
		return &packageupdaters.NpmPackageUpdater{}, nil
	default:
		return nil, fmt.Errorf("unsupported technology '%s' for component '%s'", tech, componentName)
	}
}

func inferTechnology(componentName string) techutils.Technology {
	if strings.Contains(componentName, ":") {
		return techutils.Maven
	}
	return techutils.Npm
}

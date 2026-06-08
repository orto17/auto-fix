package action

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jfrog/auto-fix/action/packageupdaters"
)

// Inputs holds the action inputs read from environment variables set by GitHub Actions.
type Inputs struct {
	ComponentName   string
	AffectedVersion string
	FixVersion      string
	GitHubToken     string
	RepoOwner       string
	RepoName        string
	BaseBranch      string
	WorkspaceDir    string
}

func ReadInputs() (Inputs, error) {
	in := Inputs{
		ComponentName:   os.Getenv("INPUT_COMPONENT_NAME"),
		AffectedVersion: os.Getenv("INPUT_AFFECTED_VERSION"),
		FixVersion:      os.Getenv("INPUT_FIX_VERSION"),
		GitHubToken:     os.Getenv("INPUT_GITHUB_TOKEN"),
		BaseBranch:      os.Getenv("INPUT_BASE_BRANCH"),
		WorkspaceDir:    os.Getenv("GITHUB_WORKSPACE"),
	}

	// GITHUB_REPOSITORY is "owner/repo"
	repo := os.Getenv("GITHUB_REPOSITORY")
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) == 2 {
		in.RepoOwner = parts[0]
		in.RepoName = parts[1]
	}

	if in.BaseBranch == "" {
		in.BaseBranch = "main"
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
	if in.WorkspaceDir != "" {
		if err := os.Chdir(in.WorkspaceDir); err != nil {
			return fmt.Errorf("failed to chdir to workspace: %w", err)
		}
		// Docker runs as a different user than the workspace owner — mark it safe for git.
		if err := gitExec("config", "--global", "--add", "safe.directory", in.WorkspaceDir); err != nil {
			return fmt.Errorf("failed to set safe.directory: %w", err)
		}
	}

	tech := DetectTechnology(in.ComponentName)
	fmt.Printf("Detected technology: %s\n", tech)

	descriptorPaths, err := LocateDescriptors(tech)
	if err != nil {
		return err
	}
	fmt.Printf("Found descriptor files: %v\n", descriptorPaths)

	details := &packageupdaters.FixDetails{
		ComponentName:   in.ComponentName,
		AffectedVersion: in.AffectedVersion,
		FixVersion:      in.FixVersion,
		DescriptorPaths: descriptorPaths,
	}

	updater := newUpdater(tech)
	if err = updater.UpdateDependency(details); err != nil {
		return fmt.Errorf("failed to update dependency: %w", err)
	}
	fmt.Printf("Successfully updated %s to %s\n", in.ComponentName, in.FixVersion)

	prURL, err := CreateFixPR(ctx, PRConfig{
		Token:           in.GitHubToken,
		Owner:           in.RepoOwner,
		Repo:            in.RepoName,
		BaseBranch:      in.BaseBranch,
		ComponentName:   in.ComponentName,
		AffectedVersion: in.AffectedVersion,
		FixVersion:      in.FixVersion,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Pull request created: %s\n", prURL)
	// Expose as action output
	_ = os.WriteFile(os.Getenv("GITHUB_OUTPUT"), []byte("pr_url="+prURL+"\n"), 0644)
	return nil
}

func newUpdater(tech Technology) packageupdaters.PackageUpdater {
	switch tech {
	case TechnologyMaven:
		return &packageupdaters.MavenPackageUpdater{}
	default:
		return &packageupdaters.NpmPackageUpdater{}
	}
}

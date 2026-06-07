package action

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

// PRConfig holds everything needed to create a fix PR.
type PRConfig struct {
	Token           string
	Owner           string
	Repo            string
	BaseBranch      string
	ComponentName   string
	AffectedVersion string
	FixVersion      string
}

// CreateFixPR commits the current working-tree changes to a new branch and opens a PR.
func CreateFixPR(ctx context.Context, cfg PRConfig) (prURL string, err error) {
	branchName := fixBranchName(cfg.ComponentName, cfg.FixVersion)

	if err = gitExec("checkout", "-b", branchName); err != nil {
		return "", fmt.Errorf("failed to create branch '%s': %w", branchName, err)
	}
	if err = gitExec("add", "."); err != nil {
		return "", fmt.Errorf("failed to stage changes: %w", err)
	}
	commitMsg := fmt.Sprintf("fix: update %s from %s to %s", cfg.ComponentName, cfg.AffectedVersion, cfg.FixVersion)
	if err = gitExec("commit", "-m", commitMsg); err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}
	if err = gitExec("push", "origin", branchName); err != nil {
		return "", fmt.Errorf("failed to push branch '%s': %w", branchName, err)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.Token})
	client := github.NewClient(oauth2.NewClient(ctx, ts))

	title := fmt.Sprintf("[Auto-Fix] Update %s to %s", cfg.ComponentName, cfg.FixVersion)
	body := fmt.Sprintf(
		"This PR was automatically created by the JFrog auto-fix action.\n\n"+
			"**Component:** `%s`\n"+
			"**Affected version:** `%s`\n"+
			"**Fix version:** `%s`\n",
		cfg.ComponentName, cfg.AffectedVersion, cfg.FixVersion,
	)

	pr, _, err := client.PullRequests.Create(ctx, cfg.Owner, cfg.Repo, &github.NewPullRequest{
		Title: github.String(title),
		Body:  github.String(body),
		Head:  github.String(branchName),
		Base:  github.String(cfg.BaseBranch),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}
	return pr.GetHTMLURL(), nil
}

func fixBranchName(componentName, fixVersion string) string {
	safe := strings.NewReplacer(":", "-", "/", "-", "@", "", " ", "-").Replace(componentName)
	return fmt.Sprintf("auto-fix/%s-%s", safe, fixVersion)
}

func gitExec(args ...string) error {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

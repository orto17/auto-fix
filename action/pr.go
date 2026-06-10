package action

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/google/go-github/v45/github"
	"github.com/jfrog/jfrog-client-go/utils/log"
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
// If a PR already exists for the same branch it logs and returns cleanly without error.
func CreateFixPR(ctx context.Context, cfg PRConfig) (prURL string, err error) {
	branchName := fixBranchName(cfg.ComponentName, cfg.FixVersion)
	log.Info(fmt.Sprintf("Creating fix PR on branch '%s' in %s/%s (base: %s)",
		branchName, cfg.Owner, cfg.Repo, cfg.BaseBranch))

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.Token})
	client := github.NewClient(oauth2.NewClient(ctx, ts))

	// Check for an existing PR before doing any git operations.
	// If the branch and PR already exist this is a no-op — log and finish cleanly.
	log.Debug(fmt.Sprintf("Checking for existing open PR on branch '%s'", branchName))
	if existingURL, err := findExistingPR(ctx, client, cfg, branchName); existingURL != "" {
		log.Info(fmt.Sprintf("Pull request already exists for branch '%s': %s — skipping creation.", branchName, existingURL))
		return existingURL, nil
	} else if err != nil {
		return "", fmt.Errorf("failed to check for existing pull request: %w", err)
	}
	log.Debug("No existing PR found — proceeding to create one")

	log.Debug(fmt.Sprintf("Creating branch '%s'", branchName))
	if err = gitExec("checkout", "-b", branchName); err != nil {
		return "", fmt.Errorf("failed to create branch '%s': %w", branchName, err)
	}

	log.Debug("Staging changes")
	if err = gitExec("add", "."); err != nil {
		return "", fmt.Errorf("failed to stage changes: %w", err)
	}

	commitMsg := fmt.Sprintf("fix: update %s from %s to %s", cfg.ComponentName, cfg.AffectedVersion, cfg.FixVersion)
	log.Debug(fmt.Sprintf("Committing with message: %s", commitMsg))
	if err = gitExec("commit", "-m", commitMsg); err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}

	log.Debug(fmt.Sprintf("Pushing branch '%s' to origin", branchName))
	if err = gitExec("push", "origin", branchName); err != nil {
		return "", fmt.Errorf("failed to push branch '%s': %w", branchName, err)
	}
	log.Info(fmt.Sprintf("Branch '%s' pushed to origin", branchName))

	title := fmt.Sprintf("[Auto-Fix] Update %s to %s", cfg.ComponentName, cfg.FixVersion)
	body := fmt.Sprintf(
		"This PR was automatically created by the JFrog auto-fix action.\n\n"+
			"**Component:** `%s`\n"+
			"**Affected version:** `%s`\n"+
			"**Fix version:** `%s`\n",
		cfg.ComponentName, cfg.AffectedVersion, cfg.FixVersion,
	)

	log.Debug(fmt.Sprintf("Opening pull request: '%s'", title))
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

// findExistingPR returns the HTML URL of an open PR from branchName, or "" if none exists.
func findExistingPR(ctx context.Context, client *github.Client, cfg PRConfig, branchName string) (string, error) {
	prs, resp, err := client.PullRequests.List(ctx, cfg.Owner, cfg.Repo, &github.PullRequestListOptions{
		State: "open",
		Head:  cfg.Owner + ":" + branchName,
		Base:  cfg.BaseBranch,
	})
	if err != nil {
		// A 404 just means the repo has no PRs yet — not a real error.
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			log.Debug("PR list returned 404 — no PRs exist in this repo yet")
			return "", nil
		}
		return "", err
	}
	if len(prs) > 0 {
		log.Debug(fmt.Sprintf("Found %d existing open PR(s) for branch '%s'", len(prs), branchName))
		return prs[0].GetHTMLURL(), nil
	}
	log.Debug(fmt.Sprintf("No open PRs found for branch '%s'", branchName))
	return "", nil
}

func fixBranchName(componentName, fixVersion string) string {
	safe := strings.NewReplacer(":", "-", "/", "-", "@", "", " ", "-").Replace(componentName)
	return fmt.Sprintf("auto-fix/%s-%s", safe, fixVersion)
}

func gitExec(args ...string) error {
	log.Debug(fmt.Sprintf("Running: git %s", strings.Join(args, " ")))
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w\n%s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

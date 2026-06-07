package packageupdaters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	ciEnv                  = "CI"
	configIgnoreScriptsEnv = "NPM_CONFIG_IGNORE_SCRIPTS"
	configAuditEnv         = "NPM_CONFIG_AUDIT"
	configFundEnv          = "NPM_CONFIG_FUND"
	configLevelEnv         = "NPM_CONFIG_LOGLEVEL"

	npmPackageLockOnlyFlag = "--package-lock-only"
	npmIgnoreScriptsFlag   = "--ignore-scripts"
	npmNoAuditFlag         = "--no-audit"
	npmLegacyPeerDepsFlag  = "--legacy-peer-deps"
	npmNoFundFlag          = "--no-fund"

	npmLockFileName          = "package-lock.json"
	npmEreresolveErrorPrefix = "ERESOLVE"
)

var npmInstallEnvVars = map[string]string{
	configIgnoreScriptsEnv: "true",
	configAuditEnv:         "false",
	configFundEnv:          "false",
	configLevelEnv:         "error",
	ciEnv:                  "true",
}

type NpmPackageUpdater struct {
	commonPackageUpdater
}

func (npm *NpmPackageUpdater) UpdateDependency(details *FixDetails) error {
	originalWd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	var errs error
	var failingDescriptors []string
	for _, descriptorPath := range details.DescriptorPaths {
		if fixErr := npm.fixAndRegenerateLock(details, descriptorPath, originalWd); fixErr != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to fix '%s' in '%s': %w", details.ComponentName, descriptorPath, fixErr))
			failingDescriptors = append(failingDescriptors, descriptorPath)
		}
	}
	if errs != nil {
		return fmt.Errorf("errors updating '%s' in [%s]: %w", details.ComponentName, strings.Join(failingDescriptors, ", "), errs)
	}
	return nil
}

func (npm *NpmPackageUpdater) fixAndRegenerateLock(details *FixDetails, descriptorPath, originalWd string) error {
	backupContent, err := npm.updatePackageJSONDescriptor(descriptorPath, details.ComponentName, details.FixVersion)
	if err != nil {
		return err
	}

	lockFilePath := filepath.Join(filepath.Dir(descriptorPath), npmLockFileName)
	if _, statErr := os.Stat(lockFilePath); os.IsNotExist(statErr) {
		return nil
	}

	return npm.withDescriptorWorkingDir(descriptorPath, originalWd, func() error {
		if err := npm.regenerateLockFileWithRetry(); err != nil {
			if rollbackErr := os.WriteFile(descriptorPath, backupContent, 0644); rollbackErr != nil {
				return fmt.Errorf("failed to rollback after lock regeneration failure: %w (original: %v)", rollbackErr, err)
			}
			return err
		}
		return nil
	})
}

func (npm *NpmPackageUpdater) regenerateLockFileWithRetry() error {
	err := npm.runNpmInstall(false)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), npmEreresolveErrorPrefix) {
		if retryErr := npm.runNpmInstall(true); retryErr != nil {
			return fmt.Errorf("npm install failed after retry with %s: %w", npmLegacyPeerDepsFlag, retryErr)
		}
		return nil
	}
	if retryErr := npm.runNpmInstall(false); retryErr != nil {
		return fmt.Errorf("npm install failed after retry: %w", retryErr)
	}
	return nil
}

func (npm *NpmPackageUpdater) runNpmInstall(useLegacyPeerDeps bool) error {
	args := []string{"install", npmPackageLockOnlyFlag, npmIgnoreScriptsFlag, npmNoAuditFlag, npmNoFundFlag}
	if useLegacyPeerDeps {
		args = append(args, npmLegacyPeerDepsFlag)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nodePackageManagerInstallTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "npm", args...)
	cmd.Env = npm.buildEnvWithOverrides(npmInstallEnvVars)
	output, err := cmd.CombinedOutput()

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("npm install timed out after %v", nodePackageManagerInstallTimeout)
	}
	if err != nil {
		return fmt.Errorf("npm install failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

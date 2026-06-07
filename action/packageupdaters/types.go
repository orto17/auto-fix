package packageupdaters

// FixDetails holds the minimal information needed to perform a dependency fix.
type FixDetails struct {
	// ComponentName is the impacted dependency name.
	// Maven format: "groupId:artifactId", NPM format: "packageName" or "@scope/packageName"
	ComponentName   string
	AffectedVersion string
	FixVersion      string
	// DescriptorPaths are the absolute paths to the descriptor files to update.
	DescriptorPaths []string
}

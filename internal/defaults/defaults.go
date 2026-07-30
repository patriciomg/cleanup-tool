// Package defaults holds shared, static default values used by multiple packages
// in cleanup-tool. It is a leaf package and should not import other project
// packages to avoid circular dependencies.
package defaults

// DepsTargets is the default list of dependency directory names used by the
// deps subcommand and by the user configuration.
func DepsTargets() []string {
	return []string{
		"node_modules",
		".pnpm",
		"vendor",
		".venv",
		"venv",
		"bower_components",
		"Pods",
		"Carthage",
		".gradle",
		".m2",
		"target",
		".tox",
		"packages",
		".nuget",
		".stack-work",
		"elm-stuff",
		"_build",
	}
}

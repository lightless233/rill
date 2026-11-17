// Package buildinfo holds the application's build identity.
package buildinfo

// Version and Commit are injected by scripts.sh; direct Go builds use the
// development defaults instead of claiming a release version.
var (
	Version = "dev"
	Commit  = "unknown"
)

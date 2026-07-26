// Package version exposes the build-time application version, shared by all
// pvs-monitoring binaries.
package version

// Version is the application version. It is overridden at build time via
//
//	-ldflags "-X github.com/dangogh/pvs-monitoring/internal/version.Version=<tag>"
//
// and defaults to "dev" for un-stamped local builds.
var Version = "dev"

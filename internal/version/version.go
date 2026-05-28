// Package version holds the build-time version string injected via -ldflags.
package version

// Version is set at build time via -ldflags "-X allstar-yaamon/internal/version.Version=vX.Y.Z".
// It falls back to "dev" when built without the flag (e.g. go run .).
var Version = "dev"

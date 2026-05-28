package config

import (
	"fmt"
	"os"
)

// CheckFilePermissions inspects sensitive files referenced by the config and
// returns a list of human-readable warning strings for any that are too permissive.
// The caller is responsible for logging them.
func CheckFilePermissions(cfg *Config, stateFile string) []string {
	var warns []string

	if cfg.configFile != "" {
		if w := warnWorldReadable(cfg.configFile, "config file"); w != "" {
			warns = append(warns, w)
		}
	}

	switch cfg.TLS.Mode {
	case "provided":
		if w := warnOwnerOnly(cfg.TLS.KeyFile, "TLS private key"); w != "" {
			warns = append(warns, w)
		}
	case "acme":
		if w := warnWorldReadable(cfg.TLS.ACMECacheDir, "ACME cache directory"); w != "" {
			warns = append(warns, w)
		}
	}

	if stateFile != "" {
		if w := warnWorldReadable(stateFile, "state file"); w != "" {
			warns = append(warns, w)
		}
	}

	return warns
}

// warnWorldReadable returns a warning if the path is readable by users other
// than the owner (others-read bit set). Group-readable config files are fine.
func warnWorldReadable(path, label string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	mode := info.Mode().Perm()
	if mode&0004 != 0 {
		return fmt.Sprintf("%s is world-readable (%04o) — restrict to 0640 or tighter: %s", label, mode, path)
	}
	return ""
}

// warnOwnerOnly returns a warning if the path is readable by anyone other than
// the owner. Used for private key files where 0600 is the expected mode.
func warnOwnerOnly(path, label string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	mode := info.Mode().Perm()
	if mode&0044 != 0 {
		return fmt.Sprintf("%s is readable by group or others (%04o) — restrict to 0600: %s", label, mode, path)
	}
	return ""
}

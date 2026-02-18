package version

import (
	"fmt"
	"runtime/debug"
)

var (
	version  = "0.25.1"
	revision = "dev"
	info, ok = debug.ReadBuildInfo()
)

// String returns scenarigo version as string.
func String() string {
	if ok {
		if info.Main.Sum != "" {
			return info.Main.Version
		}
	}
	if revision == "" {
		return fmt.Sprintf("v%s", version)
	}
	return fmt.Sprintf("v%s-%s", version, revision)
}

package version

import "fmt"

var version string = "0.1.0"
var revision string = "dev"

// String returns scenarigo version as string.
func String() string {
	if revision == "" {
		return version
	}
	return fmt.Sprintf("%s-%s", version, revision)
}

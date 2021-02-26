// NOTE: Increase the duration unit to avoid failing tests on macos-latest of GitHub Actions.
// +build darwin

package reporter

import "time"

const durationTestUnit = 1000 * time.Millisecond

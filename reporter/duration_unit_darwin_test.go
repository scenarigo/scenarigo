// NOTE: Increase the duration unit to avoid failing tests on macos-latest of GitHub Actions.
//go:build darwin
// +build darwin

package reporter

import "time"

const durationTestUnit = 50 * time.Millisecond

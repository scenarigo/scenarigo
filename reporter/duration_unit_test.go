//go:build !darwin
// +build !darwin

package reporter

import "time"

const durationTestUnit = 20 * time.Millisecond

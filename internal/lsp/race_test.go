//go:build race

package lsp

// raceSlowdown scales the time limits of tests under the race detector.
const raceSlowdown = 10

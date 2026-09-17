//go:build race

package main

// The binary under test is built with -race too, so races in igor itself are
// caught rather than only races in the harness.
const raceEnabled = true

package main

import "sync/atomic"

// session holds runtime flags parsed from command-line arguments.
// It is set once at startup and can be read from anywhere.
var sesh session

type session struct {
	NewOnly bool // --new-only: skip folders that already exist in the output
	Nuke    bool // --nuke: wipe the output folder before running

	// Counter for --new-only reporting (atomic for concurrent writes)
	FoldersSkipped atomic.Int32
}

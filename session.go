package main

import "sync/atomic"

// session holds runtime flags parsed from command-line arguments.
// It is set once at startup and can be read from anywhere.
var sesh session

type session struct {
	All   bool // --all: rebuild every folder, ignoring the manifest
	Nuke  bool // --nuke: wipe the output folder before running
	Clean bool // --clean: remove output left behind by deleted sources
	Force bool // --force: answer yes to confirmation prompts

	CLI     bool // --cli: stream plain text to stdout instead of running the TUI
	Quiet   bool // --quiet: summary and exceptions only
	Verbose bool // --verbose: add per-image and per-slice detail

	// Folders removed by --clean this run, so their manifest entries can be
	// dropped and their character siblings marked dirty.
	Pruned []string

	// Counter for reporting unchanged folders (atomic for concurrent writes)
	FoldersSkipped atomic.Int32
}

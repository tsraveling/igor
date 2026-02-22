package main

// session holds runtime flags parsed from command-line arguments.
// It is set once at startup and can be read from anywhere.
var sesh session

type session struct {
	NewOnly bool // --new-only: skip overwriting existing output files
	Nuke    bool // --nuke: wipe the output folder before running
}

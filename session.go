package main

import (
	"sort"
	"sync"
	"sync/atomic"
)

// session holds runtime flags parsed from command-line arguments.
// It is set once at startup and can be read from anywhere.
var sesh session

type session struct {
	All    bool // --all: rebuild every folder, ignoring the manifest
	Nuke   bool // --nuke: wipe the output folder before running
	Clean  bool // --clean: remove output left behind by deleted sources
	Force  bool // --force: answer yes to confirmation prompts
	Repack bool // --repack: ignore stored layout for the folders this run touches

	CLI     bool // --cli: stream plain text to stdout instead of running the TUI
	Quiet   bool // --quiet: summary and exceptions only
	Verbose bool // --verbose: add per-image and per-slice detail

	// Folders removed by --clean this run, so their manifest entries can be
	// dropped and their character frames rewritten without them.
	Pruned []string

	// Counter for reporting unchanged folders (atomic for concurrent writes)
	FoldersSkipped atomic.Int32

	// What actually hit disk. Most runs write nothing, so the summary has to
	// count writes rather than work done.
	FilesWritten   atomic.Int32
	FilesUnchanged atomic.Int32

	// Filled by the writing goroutines, read once the program has quit.
	mu      sync.Mutex
	orphans []string
	renames []string
}

// A .tres in a rebuilt folder that nothing writes any more.
func (s *session) addOrphan(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.orphans = append(s.orphans, path)
}

func (s *session) takeOrphans() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.orphans
	s.orphans = nil
	sort.Strings(out)
	return out
}

// renameLayers renames by alphabetical position, so an inserted frame shifts
// every later name. Scene references break, which has to be loud.
func (s *session) addRename(folder, from, to string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renames = append(s.renames, folder+": "+from+" -> "+to)
}

func (s *session) takeRenames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.renames
	s.renames = nil
	sort.Strings(out)
	return out
}

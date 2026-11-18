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
	All    bool // --all: rebuild every folder, still honouring its stored layout
	Nuke   bool // --nuke: wipe the output folder before running
	Clean  bool // --clean: remove output left behind by deleted sources
	Force  bool // --force: answer yes to confirmation prompts
	Repack bool // --repack: ignore stored layout and repack every folder

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

	// Character parents whose frames resource must be rewritten even though
	// no animation of theirs is in the work queue.
	FramesDirty []string

	// Filled by the writing goroutines, read once the program has quit.
	mu        sync.Mutex
	orphans   []string
	renames   []string
	warnings  []string
	reset     map[string]bool
	sheets    []string
	held      []string
	protected map[string]bool
}

// A sheet that nothing but an orphaned sprite points at. It cannot be
// reclaimed while that sprite is there, or a harmless leftover becomes a
// resource Godot cannot load, so it waits and goes with the orphan instead.
func (s *session) orphanSheet(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.protected == nil {
		s.protected = map[string]bool{}
	}
	if s.protected[path] {
		return
	}
	s.protected[path] = true
	s.held = append(s.held, path)
}

// Sheets that packing has superseded. Held until writing has finished, so an
// aborted run never leaves a .tres pointing at a file that is already gone.
func (s *session) addStaleSheet(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sheets = append(s.sheets, path)
}

func (s *session) takeStaleSheets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.sheets))
	for _, p := range s.sheets {
		if !s.protected[p] {
			out = append(out, p)
		}
	}
	s.sheets = nil
	return out
}

// Sheets kept back only because an orphan still points at them. Once that
// orphan is removed nothing references them, so they go with it.
func (s *session) heldSheets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.held...)
}

func (s *session) takeHeldSheets() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.held
	s.held = nil
	return out
}

// Warnings outlive the phase that raised them: the TUI only renders its log
// box while trimming, so anything said later would otherwise be discarded.
func (s *session) addWarning(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, msg)
}

func (s *session) takeWarnings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.warnings
	s.warnings = nil
	return out
}

// Resources whose uid igor deliberately dropped this run, so that a stale
// scene reference fails loudly. Nothing else may put that uid back.
func (s *session) markIdentityReset(resPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reset == nil {
		s.reset = map[string]bool{}
	}
	s.reset[resPath] = true
}

func (s *session) identityWasReset(resPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reset[resPath]
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

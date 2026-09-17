package main

import (
	"bufio"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

var prg *tea.Program

// Exit code chosen by the signal handler: 130 for SIGINT, 143 for SIGTERM.
var signalCode atomic.Int32

// Closed once prg is assigned, so the signal handler never reads it early.
var progReady = make(chan struct{})

const defaultConfig = `destination: ./out
source: ./in
spritesheet-size: 4096
slice-size: 1024
res-prefix: out/
rules:
  "chars/**": { mode: character }
  "env/**": { mode: env }
`

func initProject() int {
	if _, err := os.Stat("igor.yml"); err == nil {
		note("igor.yml already exists in this directory.")
		return 0
	}

	ok, code := confirmOrFail("Create igor.yml in the current directory?")
	if code != 0 {
		return code
	}
	if !ok {
		note("Cancelled.")
		return 0
	}

	if err := os.WriteFile("igor.yml", []byte(defaultConfig), 0644); err != nil {
		return fatal("Error creating igor.yml: %s", err.Error())
	}

	note("Created igor.yml.")
	return 0
}

// Where a prompt is actually visible. CLI mode keeps stdout to the phase lines
// and the summary, so prompts join the other diagnostics on stderr.
func promptWriter() *os.File {
	if !sesh.CLI && (isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())) {
		return os.Stdout
	}
	return os.Stderr
}

// Prompts unless --force was passed. A piped answer is fine; what is not fine
// is no answer at all, since the caller asked for the work to happen and
// silently skipping it would report success for work that never ran.
func confirmOrFail(question string) (bool, int) {
	if sesh.Force {
		return true, 0
	}

	fmt.Fprintf(promptWriter(), "%s [y/N] ", question)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && input == "" {
		fmt.Fprintln(promptWriter())
		return false, fatal("Error: nothing to read on stdin, so %q went unanswered. Re-run on a terminal or pass --force.", question)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes", 0
}

// Reports output left behind by deleted sources, and removes it under --clean.
// Runs before the TUI starts, since it needs the prompt. Returns whether to
// proceed, and an exit code to stop on.
func handleStale() (bool, int) {
	stale := findStale(loadManifest(), scanSourceFolders())
	if len(stale) == 0 {
		return true, 0
	}

	if !sesh.Clean {
		note("%d stale output path(s) from deleted sources. Run with --clean to remove them.", len(stale))
		return true, 0
	}

	note("These %d output path(s) no longer have sources:", len(stale))
	for _, s := range stale {
		note("  %s", s.path)
	}
	ok, code := confirmOrFail("Delete them?")
	if code != 0 {
		return false, code
	}
	if !ok {
		note("Cancelled.")
		return false, 0
	}

	removed := 0
	for _, s := range stale {
		if err := os.RemoveAll(s.path); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing %s: %s\n", s.path, err.Error())
			continue
		}
		removed++
		if sesh.CLI {
			// Deletions are audited even under --quiet.
			fmt.Fprintf(os.Stderr, "Removed %s\n", s.path)
		}
		if s.isDir {
			sesh.Pruned = append(sesh.Pruned, s.folder)
		}
	}
	note("Removed %d of %d stale path(s).", removed, len(stale))
	if removed < len(stale) {
		// Leaving stale output behind would silently poison the next build.
		return false, 1
	}
	return true, 0
}

// Fills in sesh from the command line. Returns the positional arguments, or a
// nonzero exit code if the flags do not make sense together.
func parseArgs(args []string) ([]string, int) {
	positional := []string{}
	forceCLI, forceTUI := false, false

	for _, arg := range args {
		switch arg {
		case "--all":
			sesh.All = true
		case "--nuke":
			sesh.Nuke = true
		case "--clean":
			sesh.Clean = true
		case "--force":
			sesh.Force = true
		case "--cli":
			forceCLI = true
		case "--tui":
			forceTUI = true
		case "--quiet":
			sesh.Quiet = true
		case "--verbose":
			sesh.Verbose = true
		case "--new-only":
			return nil, fatal("Error: --new-only has been removed. Incremental builds are now the default; use --all to rebuild everything.")
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, fatal("Error: unknown flag %s. Run igor --help for usage.", arg)
			}
			positional = append(positional, arg)
		}
	}

	if forceCLI && forceTUI {
		return nil, fatal("Error: --cli and --tui cannot be used together.")
	}
	if sesh.Quiet && sesh.Verbose {
		return nil, fatal("Error: --quiet and --verbose cannot be used together.")
	}

	switch {
	case forceCLI:
		sesh.CLI = true
	case forceTUI:
		sesh.CLI = false
	default:
		sesh.CLI = !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd())
	}

	if sesh.Nuke && sesh.All {
		return nil, fatal("Error: --nuke already rebuilds everything, so it cannot be used with --all.")
	}
	if sesh.Nuke && sesh.Clean {
		return nil, fatal("Error: --nuke already removes all output, so it cannot be used with --clean.")
	}

	// CLI mode cannot prompt, and both of these delete files. Refuse before
	// anything on disk has been touched rather than half way through a run.
	if sesh.CLI && !sesh.Force {
		if sesh.Nuke {
			return nil, fatal("Error: --nuke requires --force in CLI mode, since it cannot prompt.")
		}
		if sesh.Clean {
			return nil, fatal("Error: --clean requires --force in CLI mode, since it cannot prompt.")
		}
	}

	return positional, 0
}

// Quits on SIGINT or SIGTERM so a long pack can still be aborted. Headless
// bubbletea reads no input, so there is no ctrl+c keybinding to rely on.
// Killing the program unblocks run(), which owns the exit code.
func watchInterrupt() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-ch
		// Restore the default disposition so a second ctrl+c kills igor even
		// if this one does not unwind promptly.
		signal.Stop(ch)
		code := 130
		if sig == syscall.SIGTERM {
			code = 143
		}
		signalCode.Store(int32(code))

		select {
		case <-progReady:
			prg.Kill()
		default:
			// The program does not exist yet, so there is no terminal state to
			// unwind and nothing to wait for.
			fmt.Fprintln(os.Stderr, "Interrupted.")
			os.Exit(code)
		}
	}()
}

func main() {
	os.Exit(run())
}

func run() int {
	watchInterrupt()

	args := os.Args[1:]

	for _, a := range args {
		if a == "-h" || a == "--help" {
			printHelp()
			return 0
		}
	}

	positional, code := parseArgs(args)
	if code != 0 {
		return code
	}

	if len(positional) > 1 {
		return fatal("Error: expected at most one path, got %d. Run igor --help for usage.", len(positional))
	}

	dir := "."
	if len(positional) == 1 {
		if positional[0] == "init" {
			return initProject()
		}
		dir = positional[0]
	}

	if err := loadProject(dir); err != nil {
		return fatal("%s", err.Error())
	}

	emitVersion()

	// Handle --nuke: confirm, then remove the output folder
	if sesh.Nuke {
		ok, code := confirmOrFail(fmt.Sprintf("This will delete everything in %s. Are you sure?", prj.Destination))
		if code != 0 {
			return code
		}
		if !ok {
			note("Cancelled.")
			return 0
		}

		if err := os.RemoveAll(prj.Destination); err != nil {
			return fatal("Error nuking output folder: %s", err.Error())
		}
		note("Nuked %s.", prj.Destination)
	} else if proceed, code := handleStale(); !proceed {
		return code
	}

	m, _ := makeProcessModel()

	// The signal handler is ours in both modes, so a killed run reports 130 or
	// 143 rather than bubbletea's clean quit. In TUI mode ctrl+c still arrives
	// as a key press, which sets aborted.
	opts := []tea.ProgramOption{tea.WithoutSignalHandler()}
	if sesh.CLI {
		opts = append(opts, tea.WithoutRenderer(), tea.WithInput(nil))
	}
	prg = tea.NewProgram(m, opts...)
	close(progReady)

	final, err := prg.Run()
	// bubbletea reports a recovered panic as a kill, and a crash must not read
	// as an operator's ctrl+c.
	if errors.Is(err, tea.ErrProgramPanic) {
		return fatal("Error: %s", err.Error())
	}
	if errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, tea.ErrInterrupted) {
		code := int(signalCode.Load())
		if code == 0 {
			code = 130
		}
		fmt.Fprintln(os.Stderr, "Interrupted.")
		return code
	}
	if err != nil {
		return fatal("Error: %s", err.Error())
	}

	// A signal can land after Run() returns, where Kill() is a no-op, so the
	// handler's code still wins over a summary that was already printed.
	if code := int(signalCode.Load()); code != 0 {
		fmt.Fprintln(os.Stderr, "Interrupted.")
		return code
	}

	failed, aborted := false, false
	if fm, ok := final.(processModel); ok {
		failed = len(fm.exceptions) > 0
		aborted = fm.aborted
	}

	if aborted {
		fmt.Fprintln(os.Stderr, "Interrupted.")
		return 130
	}

	if !sesh.CLI {
		byes := []string{"Those who are about to die salute you.", "Happy hunting!", "Vaya con quesos.", "May the wind be always at your back.", "Later, alligator.", "Actually, Frankenstein was the doctor"}
		fmt.Printf("%s\n", byes[rand.Intn(len(byes))])
	}

	if failed {
		return 1
	}
	return 0
}

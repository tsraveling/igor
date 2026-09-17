package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

var prg *tea.Program

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
		fmt.Println("igor.yml already exists in this directory.")
		return 0
	}

	if !confirm("Create igor.yml in the current directory?") {
		fmt.Println("Cancelled.")
		return 0
	}

	if err := os.WriteFile("igor.yml", []byte(defaultConfig), 0644); err != nil {
		return fatal("Error creating igor.yml: %s", err.Error())
	}

	fmt.Println("Created igor.yml.")
	return 0
}

// Prompts unless --force was passed.
func confirm(question string) bool {
	if sesh.Force {
		return true
	}

	fmt.Printf("%s [y/N] ", question)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// Reports output left behind by deleted sources, and removes it under --clean.
// Runs before the TUI starts, since it needs the prompt. Returns false if the
// user cancelled.
func handleStale() bool {
	stale := findStale(loadManifest(), scanSourceFolders())
	if len(stale) == 0 {
		return true
	}

	if !sesh.Clean {
		fmt.Printf("%d stale output path(s) from deleted sources. Run with --clean to remove them.\n", len(stale))
		return true
	}

	fmt.Printf("These %d output path(s) no longer have sources:\n", len(stale))
	for _, s := range stale {
		fmt.Printf("  %s\n", s.path)
	}
	if !confirm("Delete them?") {
		fmt.Println("Cancelled.")
		return false
	}

	for _, s := range stale {
		if err := os.RemoveAll(s.path); err != nil {
			fmt.Printf("Error removing %s: %s\n", s.path, err.Error())
			continue
		}
		if s.isDir {
			sesh.Pruned = append(sesh.Pruned, s.folder)
		}
	}
	fmt.Printf("Removed %d stale path(s).\n", len(stale))
	return true
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

// Quits the program on SIGINT so a long pack can still be aborted. Headless
// bubbletea reads no input, so there is no ctrl+c keybinding to rely on.
func watchInterrupt() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		if prg != nil {
			prg.Kill()
		}
		fmt.Fprintln(os.Stderr, "\nInterrupted.")
		os.Exit(130)
	}()
}

func main() {
	os.Exit(run())
}

func run() int {
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

	if len(positional) > 0 && positional[0] == "init" {
		return initProject()
	}

	dir := "."
	if len(positional) > 0 {
		dir = positional[len(positional)-1]
	}

	if err := loadProject(dir); err != nil {
		return fatal("%s", err.Error())
	}

	emitVersion()

	// Handle --nuke: confirm, then remove the output folder
	if sesh.Nuke {
		if !confirm(fmt.Sprintf("This will delete everything in %s. Are you sure?", prj.Destination)) {
			fmt.Println("Cancelled.")
			return 0
		}

		if err := os.RemoveAll(prj.Destination); err != nil {
			return fatal("Error nuking output folder: %s", err.Error())
		}
		fmt.Printf("Nuked %s.\n", prj.Destination)
	} else if !handleStale() {
		return 0
	}

	m, _ := makeProcessModel()

	opts := []tea.ProgramOption{}
	if sesh.CLI {
		opts = append(opts, tea.WithoutRenderer(), tea.WithInput(nil), tea.WithoutSignalHandler())
	}
	prg = tea.NewProgram(m, opts...)

	if sesh.CLI {
		watchInterrupt()
	}

	final, err := prg.Run()
	if err != nil {
		return fatal("Error: %s", err.Error())
	}

	failed := false
	if fm, ok := final.(processModel); ok {
		failed = len(fm.exceptions) > 0
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

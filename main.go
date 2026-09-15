package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

func initProject() {
	if _, err := os.Stat("igor.yml"); err == nil {
		fmt.Println("igor.yml already exists in this directory.")
		return
	}

	fmt.Print("Create igor.yml in the current directory? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "y" && input != "yes" {
		fmt.Println("Cancelled.")
		return
	}

	if err := os.WriteFile("igor.yml", []byte(defaultConfig), 0644); err != nil {
		fmt.Printf("Error creating igor.yml: %s\n", err.Error())
		return
	}

	fmt.Println("Created igor.yml.")
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

func main() {

	if len(os.Args) > 1 && os.Args[1] == "init" {
		initProject()
		return
	}

	// Parse flags and positional args
	dir := "."
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-h", "--help":
			printHelp()
			return
		case "--all":
			sesh.All = true
		case "--nuke":
			sesh.Nuke = true
		case "--clean":
			sesh.Clean = true
		case "--force":
			sesh.Force = true
		case "--new-only":
			fmt.Println("Error: --new-only has been removed. Incremental builds are now the default; use --all to rebuild everything.")
			os.Exit(1)
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Printf("Error: unknown flag %s. Run igor --help for usage.\n", arg)
				os.Exit(1)
			}
			dir = arg
		}
	}

	if sesh.Nuke && sesh.All {
		fmt.Println("Error: --nuke already rebuilds everything, so it cannot be used with --all.")
		os.Exit(1)
	}
	if sesh.Nuke && sesh.Clean {
		fmt.Println("Error: --nuke already removes all output, so it cannot be used with --clean.")
		os.Exit(1)
	}

	err := loadProject(dir)
	if err != nil {
		fmt.Printf("%s", err.Error())
		return
	}

	// Handle --nuke: confirm, then remove the output folder
	if sesh.Nuke {
		if !confirm(fmt.Sprintf("This will delete everything in %s. Are you sure?", prj.Destination)) {
			fmt.Println("Cancelled.")
			return
		}

		if err := os.RemoveAll(prj.Destination); err != nil {
			fmt.Printf("Error nuking output folder: %s\n", err.Error())
			return
		}
		fmt.Printf("Nuked %s.\n", prj.Destination)
	} else if !handleStale() {
		return
	}

	var m tea.Model
	m, _ = makeProcessModel()

	prg = tea.NewProgram(m)
	prg.Run()

	byes := []string{"Those who are about to die salute you.", "Happy hunting!", "Vaya con quesos.", "May the wind be always at your back.", "Later, alligator.", "Actually, Frankenstein was the doctor"}
	pick := byes[rand.Intn(len(byes))]
	fmt.Printf("%s\n", pick)
}

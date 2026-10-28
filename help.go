package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

func printHelp() {
	green := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(logColor)
	flag := lipgloss.NewStyle().Foreground(secondaryColor).Bold(true)
	desc := lipgloss.NewStyle().Foreground(secondaryColor)

	fmt.Println(green.Render(igorLogo))
	fmt.Println(green.Render("  Godot asset pipeline helper"))
	fmt.Println(dim.Render("  Packs sprites, generates animations, and creates Godot resource files."))
	fmt.Println()

	fmt.Println(green.Render("USAGE"))
	fmt.Println()
	fmt.Printf("  %s %s\n", flag.Render("igor"), desc.Render("[path] [flags]"))
	fmt.Println()

	fmt.Println(green.Render("COMMANDS"))
	fmt.Println()
	fmt.Printf("  %s  %s\n", flag.Render("init"), desc.Render("Create a default igor.yml in the current directory"))
	fmt.Println()

	fmt.Println(green.Render("FLAGS"))
	fmt.Println()
	flagCol := flag.Width(16)
	fmt.Printf("  %s%s\n", flagCol.Render("--all"), desc.Render("Rebuild every folder, ignoring the build cache"))
	fmt.Printf("  %s%s\n", flagCol.Render("--clean"), desc.Render("Also remove output left behind by deleted sources (confirms first)"))
	fmt.Printf("  %s%s\n", flagCol.Render("--nuke"), desc.Render("Delete everything in the output folder before running (confirms first)"))
	fmt.Printf("  %s%s\n", flagCol.Render("--force"), desc.Render("Answer yes to confirmation prompts"))
	fmt.Printf("  %s%s\n", flagCol.Render("--cli"), desc.Render("Stream plain text to stdout instead of running the TUI"))
	fmt.Printf("  %s%s\n", flagCol.Render("--tui"), desc.Render("Force the TUI even when stdout is not a terminal"))
	fmt.Printf("  %s%s\n", flagCol.Render("--quiet"), desc.Render("CLI mode: print the summary and errors only"))
	fmt.Printf("  %s%s\n", flagCol.Render("--verbose"), desc.Render("CLI mode: add per-image and per-slice detail"))
	fmt.Printf("  %s%s\n", flagCol.Render("-h, --help"), desc.Render("Show this help message"))
	fmt.Println()

	fmt.Println(green.Render("EXAMPLES"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Rebuild whatever changed since the last run"))
	fmt.Printf("  %s\n", flag.Render("igor"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Run against a specific project folder"))
	fmt.Printf("  %s\n", flag.Render("igor ./my-project"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Rebuild everything in place, keeping Godot UIDs"))
	fmt.Printf("  %s\n", flag.Render("igor --all"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Rebuild changes and clear output for deleted sources"))
	fmt.Printf("  %s\n", flag.Render("igor --clean"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Wipe output and regenerate everything"))
	fmt.Printf("  %s\n", flag.Render("igor --nuke"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Plain text output for a build script or CI job"))
	fmt.Printf("  %s\n", flag.Render("igor --cli"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Destructive flags cannot prompt in CLI mode, so they need --force"))
	fmt.Printf("  %s\n", flag.Render("igor --cli --nuke --force"))
	fmt.Println()

	fmt.Println(green.Render("CLI MODE"))
	fmt.Println()
	fmt.Println(desc.Render("  Igor runs the TUI when stdout is a terminal and switches to plain text"))
	fmt.Println(desc.Render("  output otherwise, so piping or redirecting just works. --cli and --tui"))
	fmt.Println(desc.Render("  override the detection. Process output goes to stdout, errors to stderr."))
	fmt.Println(desc.Render("  Igor exits 1 if any errors occurred, 130 on ctrl+c, 143 on SIGTERM,"))
	fmt.Println(desc.Render("  and 0 otherwise."))
	fmt.Println()

	fmt.Println(green.Render("CONFIG"))
	fmt.Println()
	fmt.Println(desc.Render("  Igor looks for an igor.yml in the project directory. Run `igor init` to"))
	fmt.Println(desc.Render("  create one. The config sets source/destination paths, spritesheet size,"))
	fmt.Println(desc.Render("  slice size, Godot resource prefix, and folder type rules."))
	fmt.Println()
}

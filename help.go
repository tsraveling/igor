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
	fmt.Printf("  %s%s\n", flagCol.Render("--new-only"), desc.Render("Only generate new files; never overwrite existing output"))
	fmt.Printf("  %s%s\n", flagCol.Render("--nuke"), desc.Render("Delete everything in the output folder before running (confirms first)"))
	fmt.Printf("  %s%s\n", flagCol.Render("-h, --help"), desc.Render("Show this help message"))
	fmt.Println()

	fmt.Println(green.Render("EXAMPLES"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Run in the current directory"))
	fmt.Printf("  %s\n", flag.Render("igor"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Run against a specific project folder"))
	fmt.Printf("  %s\n", flag.Render("igor ./my-project"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Only write files that don't already exist"))
	fmt.Printf("  %s\n", flag.Render("igor --new-only"))
	fmt.Println()
	fmt.Printf("  %s\n", dim.Render("# Wipe output and regenerate everything"))
	fmt.Printf("  %s\n", flag.Render("igor --nuke"))
	fmt.Println()

	fmt.Println(green.Render("CONFIG"))
	fmt.Println()
	fmt.Println(desc.Render("  Igor looks for an igor.yml in the project directory. Run `igor init` to"))
	fmt.Println(desc.Render("  create one. The config sets source/destination paths, spritesheet size,"))
	fmt.Println(desc.Render("  slice size, Godot resource prefix, and folder type rules."))
	fmt.Println()
}

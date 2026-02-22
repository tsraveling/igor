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
		case "--new-only":
			sesh.NewOnly = true
		case "--nuke":
			sesh.Nuke = true
		default:
			dir = arg
		}
	}

	// Validate: --new-only and --nuke are mutually exclusive
	if sesh.NewOnly && sesh.Nuke {
		fmt.Println("Error: --new-only and --nuke cannot be used together.")
		os.Exit(1)
	}

	err := loadProject(dir)
	if err != nil {
		fmt.Printf("%s", err.Error())
		return
	}

	// Handle --nuke: confirm, then remove the output folder
	if sesh.Nuke {
		fmt.Printf("This will delete everything in %s. Are you sure? [y/N] ", prj.Destination)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input != "y" && input != "yes" {
			fmt.Println("Cancelled.")
			return
		}

		if err := os.RemoveAll(prj.Destination); err != nil {
			fmt.Printf("Error nuking output folder: %s\n", err.Error())
			return
		}
		fmt.Printf("Nuked %s.\n", prj.Destination)
	}

	var m tea.Model
	m, _ = makeProcessModel()

	prg = tea.NewProgram(m)
	prg.Run()

	byes := []string{"Those who are about to die salute you.", "Happy hunting!", "Vaya con quesos.", "May the wind be always at your back.", "Later, alligator.", "Actually, Frankenstein was the doctor"}
	pick := byes[rand.Intn(len(byes))]
	fmt.Printf("%s\n", pick)
}

package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {

	var m tea.Model
	m, _ = makeProcessModel()

	p := tea.NewProgram(m)
	p.Run()

	fmt.Printf(igorLogo)
}

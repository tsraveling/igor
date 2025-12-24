package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

func main() {

	var m tea.Model
	m, _ = makeSomeModel()

	p := tea.NewProgram(m)
	p.Run()
}

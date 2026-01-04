package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {

	// TODO: Replace this with arg[0]
	err := loadProject("./test")
	if err != nil {
		fmt.Printf("%s", err.Error())
		return
	}

	var m tea.Model
	m, _ = makeProcessModel()

	p := tea.NewProgram(m)
	p.Run()

	fmt.Printf(igorLogo)
}

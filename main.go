package main

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

var prg *tea.Program

func main() {

	// TODO: Replace this with arg[0]
	err := loadProject("./test")
	if err != nil {
		fmt.Printf("%s", err.Error())
		return
	}

	var m tea.Model
	m, _ = makeProcessModel()

	prg = tea.NewProgram(m)
	prg.Run()

	fmt.Printf(igorLogo)
}

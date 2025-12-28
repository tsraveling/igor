package main

import (
	tea "github.com/charmbracelet/bubbletea"
	// "github.com/charmbracelet/lipgloss"
)

type processModel struct {
	someValue string
}

func makeProcessModel() (processModel, tea.Cmd) {
	m := processModel{someValue: "example"}
	return m, m.Init()
}

func (m processModel) Init() tea.Cmd {
	return nil
}

func (m processModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	//	case tea.WindowSizeMsg:
	//		m.list.SetWidth(msg.Width)

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "esc", "ctrl+c":
			return m, tea.Quit
		}
	}

	// Text input gets the end of it
	// var cmd tea.Cmd
	// m.input, cmd = m.input.Update(msg)
	// return m, cmd

	return m, nil
}

func (m processModel) View() string {
	return "hello, world. source: " + prj.Source
}

// var (
// 	titleStyle = lipgloss.NewStyle().
// 			Bold(true).
// 			Foreground(lipgloss.Color("205")).
// 			MarginLeft(2)
//
// 	itemStyle = lipgloss.NewStyle().
// 			PaddingLeft(4)
// )

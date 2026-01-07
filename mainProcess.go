package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	// "github.com/charmbracelet/lipgloss"
)

type phase int

const (
	preparation phase = iota
	processing
	writing
)

type phaseCompleteMsg struct{ finished phase }

type processModel struct {
	files []imageResult
	phase phase
}

func makeProcessModel() (processModel, tea.Cmd) {
	// files := walkFiles(prj.Source)
	m := processModel{files: []imageResult{}, phase: preparation}
	return m, m.Init()
}

func (m processModel) Init() tea.Cmd {
	return walkFilesCmd(prj.Source)
}

func (m processModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	//	case tea.WindowSizeMsg:
	//		m.list.SetWidth(msg.Width)

	case preparedFileMsg:
		m.files = append(m.files, msg.file)

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
	switch m.phase {
	case preparation:
		return fmt.Sprintf("%d images", len(m.files))
	case processing:
		var b strings.Builder
		for _, r := range m.files {
			b.WriteString(r.path + "\n")
		}
		output := b.String()
		return fmt.Sprintf("%d files in %s:\n\n%s", len(m.files), prj.Source, output)
	}
	return "unsupported phase"
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

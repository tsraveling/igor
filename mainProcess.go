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
	parsing
	processing
	writing
	done
)

type exceptionCode int

const (
	unknown = iota
	errorTooLarge
)

type exception struct {
	code int
	msg  string
	file imageFile
}

type phaseCompleteMsg struct{ finished phase }

type processModel struct {
	folders      []folder
	phase        phase
	exceptions   []exception
	pendingWork  []workPiece
	activeWork   []workPiece
	finishedWork []workPiece
	failedWork   []workPiece
}

func makeProcessModel() (processModel, tea.Cmd) {
	// files := walkFiles(prj.Source)
	m := processModel{folders: []folder{}, phase: preparation}
	return m, m.Init()
}

func (m processModel) Init() tea.Cmd {
	return walkFilesCmd(prj.Source)
}

/** Moves the item of id from the first array into the second */
func move(id int, from *[]workPiece, into *[]workPiece) {
	for i, w := range *from {
		if w.ID() == id {
			*from = append((*from)[:i], (*from)[i+1:]...)
			*into = append(*into, w)
			return
		}
	}
}

func (m processModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// 1. Preparation

	case prepareCompleteMsg:
		m.folders = msg.folders
		m.phase = parsing
		return m, parseFilesCmd(m.folders)

	// 2. Parsing

	case parseCompleteMsg:
		m.pendingWork = msg.workQueue
		m.phase = processing
		return m, processWorkCmd(m.pendingWork)

	// 3. Preparation

	case startWorkMsg:
		move(msg.id, &m.pendingWork, &m.activeWork)

	case finishWorkMsg:
		move(msg.id, &m.activeWork, &m.finishedWork)

	case processingCompleteMsg:
		m.phase = done

	// -. Shared

	case exception:
		m.exceptions = append(m.exceptions, msg)

	// user input

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
		return fmt.Sprintf("%d folders", len(m.folders))
	case parsing:
		var b strings.Builder
		for _, f := range m.folders {
			var t string
			switch f.typ {
			case FolderTypeCharacter:
				t = "char"
			case FolderTypeEnv:
				t = "env"
			case FolderTypeStandard:
				t = "standard"
			}
			b.WriteString(f.path + " > " + t + ": " + f.name + "\n")
		}
		output := b.String()
		return fmt.Sprintf("%d files in %s:\n\n%s", len(m.folders), prj.Source, output)
	case processing:
		var summaryLine = fmt.Sprintf("%d pending | %d active | %d finished", len(m.pendingWork), len(m.activeWork), len(m.finishedWork))
		var b strings.Builder
		for _, w := range m.activeWork {
			switch v := w.(type) {
			case workPack:
				b.WriteString(v.f.name + " > packing\n")
			case workSlice:
				b.WriteString(v.file.filename + " > slice\n")
			}
		}
		return fmt.Sprintf("%s\n\n%s", summaryLine, b.String())
	case done:
		var b strings.Builder
		for _, w := range m.finishedWork {
			switch v := w.(type) {
			case workPack:
				b.WriteString(v.f.name + " > packed!\n")
			case workSlice:
				b.WriteString(v.file.filename + " > sliced!\n")
			}
		}
		return fmt.Sprintf("FINISHED!\n\n%s", b.String())
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

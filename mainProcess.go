package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	// "github.com/charmbracelet/lipgloss"
)

type phase int

const (
	preparation phase = iota // walk the files
	trimming                 // get the trim rect for every image (run via parallel workers)
	parsing                  // split images up by size into either packing or slicing queues
	processing               // do the actual work of packing and slicing
	writing                  // write the godot .tscn files
	done                     // fin!
)

type exceptionCode int

const (
	unknown exceptionCode = iota
	errorTooLarge
)

type exception struct {
	code exceptionCode
	msg  string
	file imageFile
}

type processModel struct {
	folders        []folder
	phase          phase
	exceptions     []exception
	numImagesTotal int

	// Trimming
	activeTrimming []string
	numTrimPending int
	numTrimDone    int

	// Processing
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

func removeString(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func (m processModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// 1. Preparation

	case prepareCompleteMsg:
		m.folders = msg.folders
		m.numImagesTotal = msg.total
		m.numTrimPending = msg.total
		m.phase = trimming
		return m, trimImagesCmd(m.folders)

	// 2. Trimming

	case startedTrimmingMsg:
		m.numTrimPending--
		m.activeTrimming = append(m.activeTrimming, msg.img)

	case finishedTrimmingMsg:
		m.numTrimDone++
		m.activeTrimming = removeString(m.activeTrimming, msg.img)

	case trimmingCompleteMsg:
		m.phase = parsing
		m.folders = msg.folders
		return m, parseFilesCmd(m.folders)

	// 3. Parsing

	case parseCompleteMsg:
		m.pendingWork = msg.workQueue
		m.phase = processing
		return m, processWorkCmd(m.pendingWork)

	// 4. Preparation

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
		return fmt.Sprintf("PARSING\n\n%d files in %s:\n\n%s", len(m.folders), prj.Source, output)
	case trimming:
		var b strings.Builder
		for _, i := range m.activeTrimming {
			b.WriteString(" - " + i + "\n")
		}
		output := b.String()
		return fmt.Sprintf("TRIMMING\n\n%d remaining --- %d done\n\n%s", m.numTrimPending, m.numTrimDone, output)
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
		return fmt.Sprintf("PROCESSING\n\n%s\n\n%s", summaryLine, b.String())
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

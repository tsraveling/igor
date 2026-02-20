package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	lipgloss "github.com/charmbracelet/lipgloss"
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
	systemError
)

type exception struct {
	code exceptionCode
	msg  string
	file *imageFile
}

func exceptionCodeName(c exceptionCode) string {
	switch c {
	case errorTooLarge:
		return "TOO LARGE"
	case systemError:
		return "SYSTEM"
	default:
		return "UNKNOWN"
	}
}

func toException(err error, imf *imageFile) exception {
	return exception{code: systemError, msg: err.Error(), file: imf}
}

type logMsg struct {
	msg string
}

type writeProgressMsg struct {
	done  int
	total int
}

type processModel struct {
	folders        []folder
	phase          phase
	exceptions     []exception
	numImagesTotal int
	logs           []string
	width          int

	// Trimming
	activeTrimming []string
	numTrimPending int
	numTrimDone    int

	// Processing
	pendingWork  []workPiece
	activeWork   []workPiece
	finishedWork []workPiece
	failedWork   []workPiece

	// Writing
	numWriteTotal int
	numWriteDone  int
	writeCh       chan writeProgressMsg
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

	// 4. Processing

	case startWorkMsg:
		move(msg.id, &m.pendingWork, &m.activeWork)

	case packWorkUpdateMsg:
		for i := range m.activeWork {
			if m.activeWork[i].ID() == msg.id {
				wp := m.activeWork[i].(workPack)
				wp.phase = msg.phase
				wp.bins = msg.bins
				m.activeWork[i] = wp
			}
		}

	case sliceWorkUpdateMsg:
		for i := range m.activeWork {
			if m.activeWork[i].ID() == msg.id {
				wp := m.activeWork[i].(workSlice)
				wp.slices = append(wp.slices, msg.slice)
				m.activeWork[i] = wp
			}
		}

	case finishWorkMsg:
		move(msg.id, &m.activeWork, &m.finishedWork)

	// 5. Writing

	case processingCompleteMsg:
		m.phase = writing
		m.numWriteTotal = len(m.finishedWork)
		m.numWriteDone = 0
		ch := make(chan writeProgressMsg)
		m.writeCh = ch
		return m, tea.Batch(startWriting(m.finishedWork, ch), waitForWriteProgress(ch))

	case writeProgressMsg:
		m.numWriteDone = msg.done
		m.numWriteTotal = msg.total
		return m, waitForWriteProgress(m.writeCh)

	case writingCompleteMsg:
		m.phase = done

	// -. Shared

	case exception:
		m.exceptions = append(m.exceptions, msg)

	case logMsg:
		m.logs = append(m.logs, msg.msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "esc", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

var phaseNames = []string{"Preparation", "Trimming", "Parsing", "Processing", "Writing", "Done"}

func (m processModel) getProgress() float64 {
	switch m.phase {
	case preparation:
		return 0.0
	case trimming:
		total := m.numTrimDone + m.numTrimPending
		if total == 0 {
			return 0.0
		}
		return float64(m.numTrimDone) / float64(total)
	case parsing:
		return 0.0
	case processing:
		total := len(m.pendingWork) + len(m.activeWork) + len(m.finishedWork)
		if total == 0 {
			return 0.0
		}
		return float64(len(m.finishedWork)) / float64(total)
	case writing:
		if m.numWriteTotal == 0 {
			return 0.0
		}
		return float64(m.numWriteDone) / float64(m.numWriteTotal)
	case done:
		return 1.0
	}
	return 0.0
}

func (m processModel) getWorkingOutput() string {
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
		for _, l := range m.logs {
			b.WriteString(l + "\n")
		}
		for _, exc := range m.exceptions {
			b.WriteString("ERR: " + exc.msg + "\n")
		}
		for _, i := range m.activeTrimming {
			b.WriteString(" & " + i + "\n")
		}
		output := b.String()
		return fmt.Sprintf("TRIMMING\n\n%d remaining --- %d done\n\n%s", m.numTrimPending, m.numTrimDone, output)
	case processing:
		var summaryLine = fmt.Sprintf("%d pending | %d active | %d finished", len(m.pendingWork), len(m.activeWork), len(m.finishedWork))
		var b strings.Builder
		for _, w := range m.activeWork {
			switch v := w.(type) {
			case workPack:
				switch v.phase {
				case calculating:
					b.WriteString(v.f.name + " > packing (calculating)\n")
				case printing:
					packString := fmt.Sprintf(" > packing (printing %d bins)\n", len(v.bins))
					b.WriteString(v.f.name + packString)
				}
			case workSlice:
				sliceString := fmt.Sprintf(" > slicing %d pieces\n", len(v.slices))
				b.WriteString(v.file.filename + sliceString)
			}
		}
		return fmt.Sprintf("PROCESSING\n\n%s\n\n%s", summaryLine, b.String())
	case writing:
		return fmt.Sprintf("WRITING\n\n%d / %d resources written", m.numWriteDone, m.numWriteTotal)
	case done:
		var b strings.Builder
		for _, w := range m.finishedWork {
			switch v := w.(type) {
			case workPack:
				packString := fmt.Sprintf(" > packed %d bins!\n", len(v.bins))
				b.WriteString(v.f.name + packString)
			case workSlice:
				sliceString := fmt.Sprintf("%s > cut %d slices!\n", v.file.filename, len(v.slices))
				b.WriteString(sliceString)
			}
		}
		return fmt.Sprintf("FINISHED!\n\n%s", b.String())
	}
	return "unsupported phase"
}

func (m processModel) View() string {
	w := boxWidth(m.width)

	// Header
	logo := logoStyle.Render(igorLogo)
	// STUB: turn phases -> phaseComponents. Bold and green when active.
	phases := phaseStyle.Render(strings.Join(phaseNames, " · "))
	// STUB: This layout actually puts phases *below* the logo. make em bottom aligned.
	header := lipgloss.JoinHorizontal(lipgloss.Bottom, logo, "  ", phases)

	// Progress
	// STUB: Make this a bit brighter
	bar := progress.New(progress.WithGradient(string(gradientColorLeft), string(gradientColorRight)))
	bar.Width = w
	prog := bar.ViewAs(m.getProgress())

	// Error box (only shown if there are exceptions)
	var errorBox string
	if len(m.exceptions) > 0 {
		var b strings.Builder
		for i, exc := range m.exceptions {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(fmt.Sprintf("[%s] %s", exceptionCodeName(exc.code), exc.msg))
			if exc.file != nil {
				b.WriteString(fmt.Sprintf("\n  %s", exc.file.path))
			}
		}
		errorBox = "\n\n" + errorBoxStyle(w).Render(b.String())
	}

	// Working output box
	outputBox := outputBoxStyle(w, m.phase == done).Render(clampLines(m.getWorkingOutput(), maxLogHeight))

	return fmt.Sprintf("%s\n\n%s%s\n\n%s\n", header, prog, errorBox, outputBox)
}

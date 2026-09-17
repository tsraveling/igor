package main

import (
	"fmt"
	"os"
	"path/filepath"
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

// A non-fatal problem worth surfacing, such as an image that could not be
// trimmed. Unlike an exception it does not fail the run.
type warnMsg struct {
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
	manifestNote  string
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
		folders, images, skipped := filterDirty(msg.folders)
		sesh.FoldersSkipped.Store(int32(skipped))
		m.folders = folders
		m.numImagesTotal = images
		m.numTrimPending = images
		emitPhase(preparation, "%s, %s, %d unchanged skipped", plural(len(folders), "folder"), plural(images, "image"), skipped)
		m.phase = trimming
		emitPhase(trimming, "%s", plural(images, "image"))
		return m, trimImagesCmd(m.folders)

	// 2. Trimming

	case startedTrimmingMsg:
		m.numTrimPending--
		m.activeTrimming = append(m.activeTrimming, msg.img)

	case finishedTrimmingMsg:
		m.numTrimDone++
		m.activeTrimming = removeString(m.activeTrimming, msg.img)
		emitDetail(trimming, "trimmed %s", msg.img)

	case trimmingCompleteMsg:
		emitPhase(trimming, "done, %d trimmed", m.numTrimDone)
		m.phase = parsing
		m.folders = msg.folders
		for _, f := range m.folders {
			emitDetail(parsing, "%s: %s", f.path, folderTypeName(f.config.typ))
		}
		return m, parseFilesCmd(m.folders)

	// 3. Parsing

	case parseCompleteMsg:
		m.pendingWork = msg.workQueue
		emitPhase(parsing, "%s", plural(len(msg.workQueue), "work piece"))
		m.phase = processing
		return m, processWorkCmd(m.pendingWork)

	// 4. Processing

	case startWorkMsg:
		move(msg.id, &m.pendingWork, &m.activeWork)

	case packWorkUpdateMsg:
		emitDetail(processing, "%s: printing %s", activeWorkName(m.activeWork, msg.id), plural(len(msg.bins), "bin"))
		for i := range m.activeWork {
			if m.activeWork[i].ID() == msg.id {
				wp := m.activeWork[i].(workPack)
				wp.phase = msg.phase
				wp.bins = msg.bins
				m.activeWork[i] = wp
			}
		}

	case sliceWorkUpdateMsg:
		emitDetail(processing, "%s: cut slice %s", activeWorkName(m.activeWork, msg.id), msg.slice.path)
		for i := range m.activeWork {
			if m.activeWork[i].ID() == msg.id {
				wp := m.activeWork[i].(workSlice)
				wp.slices = append(wp.slices, msg.slice)
				m.activeWork[i] = wp
			}
		}

	case finishWorkMsg:
		move(msg.id, &m.activeWork, &m.finishedWork)
		// The moved copy carries the bins and slices accumulated by the update
		// messages; the one on the msg does not.
		if n := len(m.finishedWork); n > 0 {
			emitPhase(processing, "%s", workResultLine(m.finishedWork[n-1]))
		}

	// 5. Writing

	case processingCompleteMsg:
		m.phase = writing
		emitPhase(writing, "%s", plural(len(m.finishedWork), "resource"))
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
		if mf, ok := buildManifest(m.folders, m.finishedWork, m.exceptions); ok {
			if err := mf.save(); err != nil {
				m.manifestNote = "Could not write the build cache: " + err.Error()
			}
		} else {
			m.manifestNote = "Build cache not updated because of unattributable errors; the next run will rebuild everything."
		}
		emitSummary(m.summaryLine())
		if m.manifestNote != "" && sesh.CLI {
			fmt.Fprintln(os.Stderr, m.manifestNote)
		}
		return m, tea.Quit

	// -. Shared

	case exception:
		m.exceptions = append(m.exceptions, msg)
		emitException(msg)

	case logMsg:
		m.logs = append(m.logs, msg.msg)

	case warnMsg:
		m.logs = append(m.logs, msg.msg)
		emitWarning(msg.msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.phase == done {
				return m, tea.Quit
			}
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
			switch f.config.typ {
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

		if sesh.Nuke {
			nukeStyle := lipgloss.NewStyle().Foreground(warningColor).Bold(true)
			b.WriteString(nukeStyle.Render("Nuked the output directory before writing!"))
			b.WriteString("\n\n")
		}

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

		if skipped := sesh.FoldersSkipped.Load(); skipped > 0 {
			b.WriteString(fmt.Sprintf("\n%d unchanged folders skipped", skipped))
		}

		if m.manifestNote != "" {
			warnStyle := lipgloss.NewStyle().Foreground(warningColor).Bold(true)
			b.WriteString("\n" + warnStyle.Render(m.manifestNote))
		}

		return fmt.Sprintf("FINISHED!\n\n%s", b.String())
	}
	return "unsupported phase"
}

func (m processModel) View() string {
	if sesh.CLI {
		return ""
	}

	w := boxWidth(m.width)

	// Header
	logo := logoStyle.Height(4).Render(igorLogo)

	pending := false
	phaseElements := []string{}
	for p := preparation; p <= done; p++ {
		if m.phase == p {
			phaseElements = append(phaseElements, phaseStyle.Render(phaseNames[p]))
			pending = true
		} else if pending {
			phaseElements = append(phaseElements, phasePendingStyle.Render(phaseNames[p]))
		} else {
			phaseElements = append(phaseElements, phaseDoneStyle.Render(phaseNames[p]))
		}
	}
	phaseLine := lipgloss.PlaceVertical(4, lipgloss.Bottom, strings.Join(phaseElements, " . "))
	header := lipgloss.JoinHorizontal(lipgloss.Bottom, logo, "  ", phaseLine)

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

	hint := "working . ctrl+c to abort"
	if m.phase == done {
		hint = "esc or ctrl+c to quit"
	}
	footer := lipgloss.NewStyle().Foreground(logColor).Render(hint)

	return fmt.Sprintf("%s\n\n%s%s\n\n%s\n%s\n", header, prog, errorBox, outputBox, footer)
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func folderTypeName(t folderType) string {
	switch t {
	case FolderTypeCharacter:
		return "char"
	case FolderTypeEnv:
		return "env"
	default:
		return "standard"
	}
}

// Names an in-flight work piece, for verbose logging.
func activeWorkName(active []workPiece, id int) string {
	for _, w := range active {
		if w.ID() != id {
			continue
		}
		switch v := w.(type) {
		case workPack:
			return v.f.path
		case workSlice:
			return filepath.Join(v.f.path, v.file.filename)
		}
	}
	return "?"
}

func workResultLine(w workPiece) string {
	switch v := w.(type) {
	case workPack:
		return fmt.Sprintf("packed %s: %s", v.f.path, plural(len(v.bins), "bin"))
	case workSlice:
		return fmt.Sprintf("cut %s: %s", filepath.Join(v.f.path, v.file.filename), plural(len(v.slices), "slice"))
	}
	return "finished work"
}

func (m processModel) summaryLine() string {
	packed, sliced := 0, 0
	for _, w := range m.finishedWork {
		switch w.(type) {
		case workPack:
			packed++
		case workSlice:
			sliced++
		}
	}
	return fmt.Sprintf("%d folders, %d packed, %d sliced, %d skipped, %d errors",
		len(m.folders), packed, sliced, sesh.FoldersSkipped.Load(), len(m.exceptions))
}

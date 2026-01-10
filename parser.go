package main

import tea "github.com/charmbracelet/bubbletea"

type someMsg struct {
	file imageFile
}

type parseCompleteMsg struct{}

func parseFilesCmd(images []imageFile) tea.Cmd {
	return func() tea.Msg {
		// STUB: Next, slice the flat image array into pieces of work:
		// 1. folders to pack
		// 2. large images to slice
		return nil
	}
}

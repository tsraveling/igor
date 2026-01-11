package main

import tea "github.com/charmbracelet/bubbletea"

type someMsg struct {
	file imageFile
}

type parseCompleteMsg struct {
	workQueue []any
}

/*
The purpose of this cmd is to turn folders into work that can
be chunked and paralellized
*/
func parseFilesCmd(folders []folder) tea.Cmd {
	return func() tea.Msg {
		q := []any{}

		// STUB: Next, slice the flat image array into pieces of work:
		// 1. folders to pack
		// 2. large images to slice

		return nil
	}
}

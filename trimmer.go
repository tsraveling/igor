package main

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type startedTrimmingMsg struct {
	id  int
	img imageFile
}

type finishedTrimmingMsg struct {
	id  int
	img imageFile
}

type trimmingCompleteMsg struct{}

func trimImagesCmd(folders []folder) tea.Cmd {
	return func() tea.Msg {

		// STUB: Flat out all of the images here or otherwise iterate through these

		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup

		index := 0
		for _, f := range folders {
			for _, i := range f.files {
				wg.Add(1)
				sem <- struct{}{} // This uses zero memory
				go func(id int, img imageFile) {
					defer wg.Done()
					defer func() { <-sem }()
					prg.Send(startedTrimmingMsg{id, img})
					// STUB: Do actual trimming here!
					prg.Send(finishedTrimmingMsg{id, img})
				}(index, i)
				index++
			}
		}

		wg.Wait()
		return processingCompleteMsg{}
	}
}

package main

import (
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type startedTrimmingMsg struct {
	id  int
	img string
}

type finishedTrimmingMsg struct {
	id  int
	img string
}

type trimmingCompleteMsg struct{ folders []folder }

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
					prg.Send(startedTrimmingMsg{id, f.path + img.filename})
					// STUB: Do actual trimming here!
					time.Sleep(500 * time.Millisecond)
					prg.Send(finishedTrimmingMsg{id, f.path + img.filename})
				}(index, i)
				index++
			}
		}

		wg.Wait()
		return trimmingCompleteMsg{folders}
	}
}

func getTrimRect(f imageFile) (*rect, error) {
	img, err := f.load()
	if err != nil {
		return nil, err
	}
	// Do something with img
	return nil, nil
}

package main

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type startWorkMsg struct {
	id   int
	work any
}

type finishWorkMsg struct {
	id   int
	work any
}

type workCompleteMsg struct{}

const maxWorkers = 8

func processWorkCmd(work []any) tea.Cmd {
	return func() tea.Msg {

		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup

		for _, w := range work {
			wg.Add(1)
			sem <- struct{}{} // This uses zero memory

			switch v := w.(type) {
			case workPack:
				go func(id int, p workPack) {
					defer wg.Done()
					defer func() { <-sem }()
					prg.Send(startWorkMsg{id: id, work: p})
					pack(p)
					prg.Send(finishWorkMsg{id: id, work: p})
				}(v.id, v)
			case workSlice:
				go func(id int, s workSlice) {
					defer wg.Done()
					defer func() { <-sem }()
					prg.Send(startWorkMsg{id: id, work: s})
					slice(s)
					prg.Send(finishWorkMsg{id: id, work: s})
				}(v.id, v)
			}
		}

		wg.Wait()
		return workCompleteMsg{}
	}
}

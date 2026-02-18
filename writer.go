package main

import (
	"path/filepath"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type writingCompleteMsg struct{}

func writeResourcesCmd(work []workPiece) tea.Cmd {
	return func() tea.Msg {
		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup

		for _, w := range work {
			switch v := w.(type) {
			case workPack:
				wg.Add(1)
				sem <- struct{}{}
				go func(wp workPack) {
					defer wg.Done()
					defer func() { <-sem }()
					writeTres(wp)
				}(v)
			case workSlice:
				wg.Add(1)
				sem <- struct{}{}
				go func(ws workSlice) {
					defer wg.Done()
					defer func() { <-sem }()
					writeSliceTscn(ws)
				}(v)
			}
		}

		wg.Wait()

		// Generate SpriteFrames for character folders
		charGroups := map[string][]workPack{}
		for _, w := range work {
			if wp, ok := w.(workPack); ok && wp.f.typ == FolderTypeCharacter {
				parentPath := filepath.Dir(wp.f.path)
				charGroups[parentPath] = append(charGroups[parentPath], wp)
			}
		}
		for parentPath, packs := range charGroups {
			charName := filepath.Base(parentPath)
			writeSpriteFrames(charName, parentPath, packs)
		}

		return writingCompleteMsg{}
	}
}

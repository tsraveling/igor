package main

import (
	"path/filepath"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

type writingCompleteMsg struct{}

func startWriting(work []workPiece, mf manifest, ch chan writeProgressMsg) tea.Cmd {
	return func() tea.Msg {
		total := len(work)
		var done atomic.Int32
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
					ch <- writeProgressMsg{done: int(done.Add(1)), total: total}
				}(v)
			case workSlice:
				wg.Add(1)
				sem <- struct{}{}
				go func(ws workSlice) {
					defer wg.Done()
					defer func() { <-sem }()
					writeSliceTscn(ws)
					ch <- writeProgressMsg{done: int(done.Add(1)), total: total}
				}(v)
			}
		}

		wg.Wait()

		// Generate SpriteFrames for every character touched this run. The
		// contents come from the manifest, so a rebuilt animation no longer
		// has to drag its siblings into the work queue to stay in the file.
		charParents := map[string]bool{}
		for _, w := range work {
			if wp, ok := w.(workPack); ok && wp.f.config.typ == FolderTypeCharacter {
				charParents[filepath.Dir(wp.f.path)] = true
			}
		}
		for _, pruned := range sesh.Pruned {
			if getFolderConfig(pruned).typ == FolderTypeCharacter {
				charParents[filepath.Dir(pruned)] = true
			}
		}
		for _, parent := range sesh.FramesDirty {
			charParents[parent] = true
		}
		for parentPath := range charParents {
			writeSpriteFrames(filepath.Base(parentPath), parentPath, mf)
		}

		// Generate reassembly .tscn for env folders
		envPacks := map[string][]workPack{}
		envSlices := map[string][]workSlice{}
		for _, w := range work {
			switch v := w.(type) {
			case workPack:
				if v.f.config.typ == FolderTypeEnv {
					envPacks[v.f.path] = append(envPacks[v.f.path], v)
				}
			case workSlice:
				if v.f.config.typ == FolderTypeEnv {
					envSlices[v.f.path] = append(envSlices[v.f.path], v)
				}
			}
		}
		envFolders := map[string]bool{}
		for p := range envPacks {
			envFolders[p] = true
		}
		for p := range envSlices {
			envFolders[p] = true
		}
		for folderPath := range envFolders {
			writeEnvTscn(folderPath, envPacks[folderPath], envSlices[folderPath])
		}

		close(ch)
		return nil // writingCompleteMsg comes from waitForWriteProgress reading a closed channel
	}
}

func waitForWriteProgress(ch chan writeProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return writingCompleteMsg{}
		}
		return msg
	}
}

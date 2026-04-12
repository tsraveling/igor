package main

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

type parseCompleteMsg struct {
	workQueue []workPiece
}

/*
The purpose of this cmd is to turn folders into work that can
be chunked and paralellized
*/
func parseFilesCmd(folders []folder) tea.Cmd {
	return func() tea.Msg {
		q := []workPiece{}

		for _, f := range folders {
			if sesh.NewOnly {
				outputDir := filepath.Join(prj.Destination, f.path)
				if _, err := os.Stat(outputDir); err == nil {
					sesh.FoldersSkipped.Add(1)
					continue
				}
			}

			toPack := []imageFile{}
			for _, i := range f.files {
				if f.config.typ != FolderTypeCharacter && (i.trim.w > prj.SliceSize || i.trim.h > prj.SliceSize) {
					q = append(q, workSlice{f: f, file: i, id: len(q)})
				} else {
					toPack = append(toPack, i)
				}
			}
			if len(toPack) > 0 {
				q = append(q, workPack{f: f, id: len(q), files: toPack})
			}
		}

		prg.Send(parseCompleteMsg{workQueue: q})
		return nil
	}
}

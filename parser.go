package main

import (
	"fmt"

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
			toPack := []imageFile{}
			for _, i := range f.files {
				if i.trimRect.w > prj.SliceSize || i.trimRect.h > prj.SliceSize {
					if f.typ != FolderTypeCharacter {
						q = append(q, workSlice{f: f, file: i, id: len(q)})
					} else {
						prg.Send(exception{code: errorTooLarge, msg: fmt.Sprintf("%s has dimensions %d, %d, which is larger than slice size %d -- not allowed in a character type folder!", f.name, i.w, i.h, prj.SliceSize)})
					}
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

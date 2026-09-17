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
				if f.config.typ != FolderTypeCharacter && (i.trim.w > prj.SliceSize || i.trim.h > prj.SliceSize) {
					q = append(q, workSlice{f: f, file: i, id: len(q)})
					continue
				}

				// Caught here rather than in the packer, which would already
				// have written a spritesheet missing this frame.
				if i.trim.w > prj.SpritesheetSize || i.trim.h > prj.SpritesheetSize {
					prg.Send(exception{
						code: errorTooLarge,
						file: &i,
						msg: fmt.Sprintf("%s has trimmed dimensions %d x %d, which exceeds spritesheet size %d",
							i.filename, i.trim.w, i.trim.h, prj.SpritesheetSize),
					})
					continue
				}

				toPack = append(toPack, i)
			}
			if len(toPack) > 0 {
				q = append(q, workPack{f: f, id: len(q), files: toPack})
			}
		}

		prg.Send(parseCompleteMsg{workQueue: q})
		return nil
	}
}

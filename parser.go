package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// A divided image contributes one virtual imageFile per piece, each pointing
// back at the same PNG with its own sub-rect, so nothing is cut to disk before
// packing. A lone piece keeps the source name; several get reading-order
// suffixes.
func expandPieces(i imageFile) []imageFile {
	if len(i.pieces) == 0 {
		return []imageFile{i}
	}

	base := i.baseName()
	out := make([]imageFile, 0, len(i.pieces))
	for n, p := range i.pieces {
		piece := i
		piece.pieces = nil
		piece.trim = trimRect{rect: p, mR: i.w - p.r(), mB: i.h - p.b()}
		if len(i.pieces) > 1 {
			piece.outName = fmt.Sprintf("%s_%02d", base, n+1)
		}
		out = append(out, piece)
	}
	return out
}

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
			if f.config.divideAbove > 0 && f.config.typ == FolderTypeCharacter {
				warn(f.path + ": divideAbove ignored in character mode, frames must stay whole")
			}

			toPack := []imageFile{}
			for _, src := range f.files {
				for _, i := range expandPieces(src) {
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
								i.baseName(), i.trim.w, i.trim.h, prj.SpritesheetSize),
						})
						continue
					}

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

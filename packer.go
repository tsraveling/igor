package main

import "time"

func pack(w workPack) {

	time.Sleep(300 * time.Millisecond)
	bins := maxRects(w.files)
	// STUB: Print
}

func maxRects(files []imageFile) []spriteBin {
	firstBin := spriteBin{freeRects: []freeRect{freeRect{w: prj.SpritesheetSize, h: prj.SpritesheetSize}}}
	bins := []spriteBin{firstBin}

	for i, img := range files {
		for _, bin := range bins {
			for _, fr := range bin.freeRects {
				// STUB: Decide free rectangle to pack img.trimRect into
				// STUB: If none is found, start a new bin
				// STUB: Set the spriteRect to the bottom left of the free rect
				// STUB: split the remainder into new freeRects.
			}
		}
	}
	return bins
}

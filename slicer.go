package main

import "time"

type sliceWorkUpdateMsg struct {
	id    int
	slice imgSlice
}

func slice(w workSlice) {
	step := prj.SliceSize
	rx, ry := w.file.trim.x, w.file.trim.y
	slicer := rect{x: rx, y: ry, w: step, h: step}
	for slicer.b() < w.file.trim.b() {
		for slicer.r() < w.file.trim.r() {
			// STUB: Slice into file
			// Then move the slicer
			time.Sleep(300 * time.Millisecond)
			prg.Send(sliceWorkUpdateMsg{id: w.id, slice: imgSlice{rect: slicer, path: "???"}})
			slicer.x += step
		}
		slicer.y += step
		slicer.x = w.file.trim.x
	}
}

package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

type sliceWorkUpdateMsg struct {
	id    int
	slice imgSlice
}

func slice(w workSlice) {
	src, err := w.file.load()
	if err != nil {
		prg.Send(toException(err, &w.file))
		return
	}

	// Build output directory: {DEST}/{path}/{basename}/
	basename := strings.TrimSuffix(w.file.filename, filepath.Ext(w.file.filename))
	outDir := filepath.Join(prj.Destination, w.file.path, basename)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		prg.Send(toException(err, &w.file))
		return
	}

	step := prj.SliceSize
	trim := w.file.trim
	sliceIdx := 0

	slicer := rect{x: trim.x, y: trim.y, w: step, h: step}
	for slicer.y < trim.b() {
		slicer.x = trim.x
		for slicer.x < trim.r() {
			// Clamp to the trim bounds
			clampedW := min(step, trim.r()-slicer.x)
			clampedH := min(step, trim.b()-slicer.y)

			// Crop the slice from the source image
			sliceImg := image.NewRGBA(image.Rect(0, 0, clampedW, clampedH))
			srcRect := image.Rect(slicer.x, slicer.y, slicer.x+clampedW, slicer.y+clampedH)
			draw.Draw(sliceImg, sliceImg.Bounds(), src, srcRect.Min, draw.Src)

			// Save as {basename}_x{NN}.png
			sliceIdx++
			filename := fmt.Sprintf("%s_x%02d.png", basename, sliceIdx)
			slicePath := filepath.Join(outDir, filename)

			if err := saveSlice(sliceImg, slicePath); err != nil {
				prg.Send(toException(err, &w.file))
				continue
			}

			actualSlicer := rect{x: slicer.x, y: slicer.y, w: clampedW, h: clampedH}
			prg.Send(sliceWorkUpdateMsg{id: w.id, slice: imgSlice{rect: actualSlicer, path: slicePath}})

			slicer.x += step
		}
		slicer.y += step
	}
}

func saveSlice(img image.Image, path string) error {
	if sesh.NewOnly {
		if _, err := os.Stat(path); err == nil {
			return nil // already exists, skip
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

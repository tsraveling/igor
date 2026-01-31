package main

import (
	"fmt"
	"image"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type startedTrimmingMsg struct {
	id  int
	img string
}

type finishedTrimmingMsg struct {
	id  int
	img string
	err error
}

type trimmingCompleteMsg struct{ folders []folder }

func trimImagesCmd(folders []folder) tea.Cmd {
	return func() tea.Msg {

		// STUB: Flat out all of the images here or otherwise iterate through these
		prg.Send(logMsg{"it begins"})

		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup
		var mu sync.Mutex

		index := 0
		for fi, f := range folders {
			for ii, img := range f.files {
				wg.Add(1)
				sem <- struct{}{} // This uses zero memory
				go func(id int, folderIdx int, imageIdx int, img imageFile) {
					defer wg.Done()
					defer func() { <-sem }()
					prg.Send(logMsg{"starting " + img.filename})

					prg.Send(startedTrimmingMsg{id, f.path + img.filename})

					tR, err := getTrimRect(img)
					if err != nil {
						// Handle error however you want
						prg.Send(logMsg{"ERR: " + err.Error()})
						prg.Send(finishedTrimmingMsg{id: id, img: img.path + img.filename, err: err})
						return
					}

					prg.Send(logMsg{tR.toStr()})

					mu.Lock()
					folders[folderIdx].files[imageIdx].trim = *tR
					mu.Unlock()

					prg.Send(finishedTrimmingMsg{id: id, img: f.path + img.filename})
				}(index, fi, ii, img)
				index++
			}
		}

		wg.Wait()
		return trimmingCompleteMsg{folders}
	}
}

func getTrimRect(f imageFile) (*trimRect, error) {
	img, err := f.load()
	if err != nil {
		return nil, err
	}
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		return nil, fmt.Errorf("%s: expected NRGBA, got %T", f.path, img)
	}

	// 1. Start with the top
	minY := -1
	for y := 0; y < f.h && minY < 0; y++ {
		row := nrgba.Pix[y*nrgba.Stride : y*nrgba.Stride+f.w*4]
		for x := 0; x < f.w; x++ {
			// Each pixel is 4 bytes, the last one is transparency so grab that.
			// If any pixel is more than zero there's something here, this is the top
			// of the image.
			if row[x*4+3] > 0 {
				minY = y
				break
			}
		}
	}

	// If the entire image is transparent, return an empty rect
	if minY < 0 {
		return nil, fmt.Errorf("%s: Image completely empty", f.path)
	}

	// 2. Find bottom edge (scan backwards)
	maxY := f.h - 1
outer:
	for y := f.h - 1; y > minY; y-- {
		row := nrgba.Pix[y*nrgba.Stride : y*nrgba.Stride+f.w*4]
		for x := 0; x < f.w; x++ {
			if row[x*4+3] > 0 {
				maxY = y
				break outer
			}
		}
	}

	// 3. Now we know the top and bottom limits. Scan in on those rows.
	minX, maxX := f.w, 0
	for y := minY; y <= maxY; y++ {
		row := nrgba.Pix[y*nrgba.Stride : y*nrgba.Stride+f.w*4]

		// Move minX left every time we find a closer pixel
		for x := 0; x < minX; x++ {
			if row[x*4+3] > 0 {
				minX = x
				break
			}
		}

		// Move minX right every time we find a closer pixel
		for x := f.w - 1; x > maxX; x-- {
			if row[x*4+3] > 0 {
				maxX = x
				break
			}
		}
	}

	// 4. Compose and return the trimmed rect
	return &trimRect{
		rect: rect{
			x: minX,
			y: minY,
			w: (maxX - minX) + 1,
			h: (maxY - minY) + 1,
		},
		mR: f.w - maxX,
		mB: f.h - maxY,
	}, nil
}

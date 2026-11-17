package main

import (
	"fmt"
	"image"
	"path/filepath"
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

		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup
		var mu sync.Mutex

		index := 0
		for fi, f := range folders {
			for ii, img := range f.files {
				wg.Add(1)
				sem <- struct{}{} // This uses zero memory
				go func(id int, folderIdx int, imageIdx int, img imageFile, cfg folderConfig) {
					defer wg.Done()
					defer func() { <-sem }()
					name := filepath.Join(prj.Source, f.path, img.filename)
					prg.Send(startedTrimmingMsg{id, name})

					tR, pieces, err := analyze(img, cfg)
					if err != nil {
						// An untrimmed image would pack at zero size and vanish
						// from the atlas, so this fails the run.
						prg.Send(exception{code: systemError, file: &img, msg: "could not trim: " + err.Error()})
						prg.Send(finishedTrimmingMsg{id: id, img: name, err: err})
						return
					}

					// CLI mode reports trimming through finishedTrimmingMsg, and
					// never renders logMsg, so skip the round trip there.
					if !sesh.CLI {
						prg.Send(logMsg{img.filename + " " + tR.toStr()})
					}

					for _, p := range pieces {
						if p.w < minItemSize || p.h < minItemSize {
							warn(fmt.Sprintf("%s: item at %d,%d is only %dx%d", name, p.x, p.y, p.w, p.h))
						}
					}

					mu.Lock()
					folders[folderIdx].files[imageIdx].trim = *tR
					folders[folderIdx].files[imageIdx].pieces = pieces
					mu.Unlock()

					prg.Send(finishedTrimmingMsg{id: id, img: name})
				}(index, fi, ii, img, f.config)
				index++
			}
		}

		wg.Wait()
		return trimmingCompleteMsg{folders}
	}
}

// One load serves both the trim rect and, where a rule asks for it, the
// divideAbove pass; decoding a 9000x8000 PNG twice is not worth the tidier
// split.
func analyze(f imageFile, cfg folderConfig) (*trimRect, []rect, error) {
	img, err := f.load()
	if err != nil {
		return nil, nil, err
	}
	nrgba, ok := img.(*image.NRGBA)
	if !ok {
		return nil, nil, fmt.Errorf("%s: expected NRGBA, got %T", filepath.Join(f.path, f.filename), img)
	}

	tR, err := trimRectOf(f, nrgba)
	if err != nil {
		return nil, nil, err
	}
	if !cfg.dividing() {
		return tR, nil, nil
	}

	pieces := divide(nrgba, tR.rect, cfg.divideAbove, cfg.alphaThreshold)
	if len(pieces) == 0 {
		return nil, nil, fmt.Errorf("%s: no pixels above alphaThreshold %d",
			filepath.Join(f.path, f.filename), cfg.alphaThreshold)
	}
	return tR, pieces, nil
}

func trimRectOf(f imageFile, nrgba *image.NRGBA) (*trimRect, error) {

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
		return nil, fmt.Errorf("%s: image is completely empty", filepath.Join(f.path, f.filename))
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
		mR: f.w - maxX - 1,
		mB: f.h - maxY - 1,
	}, nil
}

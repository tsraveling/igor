package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func imgRectFor(r rect) image.Rectangle {
	return image.Rect(r.x, r.y, r.x+r.w, r.y+r.h)
}

func renderBins(bins []spriteBin, imgs []imageFile) []error {

	errors := []error{}
	if len(imgs) == 0 {
		return append(errors, fmt.Errorf("Sprite list was empty, aborting write."))
	}
	for bi, bin := range bins {
		// 1. First calculate spritesheet size
		maxX, maxY := 0, 0
		for _, p := range bin.rects {
			rx := p.rect.x + p.rect.w
			ry := p.rect.y + p.rect.h
			if rx > maxX {
				maxX = rx
			}
			if ry > maxY {
				maxY = ry
			}
		}

		sheetW, sheetH := prj.SpritesheetSize, prj.SpritesheetSize
		for sheetW/2 > maxX {
			sheetW = sheetW / 2
		}
		for sheetH/2 > maxY {
			sheetH = sheetH / 2
		}

		sheet := image.NewRGBA(image.Rect(0, 0, sheetW, sheetH))

		// 2. Then render
		trgFolder := ""
		for _, p := range bin.rects {
			img := imgs[p.i]
			if trgFolder == "" {
				trgFolder = img.targetFolderPath()
			}
			src, err := img.load()
			if err != nil {
				errors = append(errors, err)
				break
			}
			srcRect := imgRectFor(img.trim.rect)
			dstRect := imgRectFor(p.rect)
			draw.Draw(sheet, dstRect, src, srcRect.Min, draw.Src)
		}
		if trgFolder == "" {
			errors = append(errors, fmt.Errorf("Failed to get target folder for a bin with %d rects", len(bin.rects)))
			break
		}
		err := savePng(sheet, trgFolder, bi, len(bins))
		if err != nil {
			errors = append(errors, err)
			break
		}
	}
	return errors
}

func savePng(img image.Image, path string, i int, total int) error {
	// create the directory and any parents
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	// get the folder name for the filename
	add := ""
	if total > 0 {
		add = fmt.Sprintf("_%02d", i)
	}
	name := filepath.Base(path) + add + ".png"
	fullPath := filepath.Join(path, name)

	if sesh.NewOnly {
		if _, err := os.Stat(fullPath); err == nil {
			return nil // already exists, skip
		}
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

package main

import (
	"bytes"
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

func renderBins(bins []spriteBin, imgs []imageFile, trgFolder string) []error {

	errors := []error{}
	if len(imgs) == 0 {
		return append(errors, fmt.Errorf("Sprite list was empty, aborting write."))
	}

	for bi, bin := range bins {
		// 1. First calculate spritesheet size
		maxX, maxY := 0, 0
		for _, p := range bin.rects {
			maxX = max(maxX, p.rect.r())
			maxY = max(maxY, p.rect.b())
		}

		sheet := image.NewRGBA(image.Rect(0, 0, sheetDim(maxX), sheetDim(maxY)))

		// 2. Then render
		for _, p := range bin.rects {
			img := imgs[p.i]
			src, err := img.load()
			if err != nil {
				errors = append(errors, err)
				break
			}
			srcRect := imgRectFor(img.trim.rect)
			dstRect := imgRectFor(p.rect)
			draw.Draw(sheet, dstRect, src, srcRect.Min, draw.Src)
		}

		if err := savePng(sheet, trgFolder, bi); err != nil {
			errors = append(errors, err)
			break
		}
	}

	if err := removeOrphanedBins(trgFolder, len(bins)); err != nil {
		errors = append(errors, err)
	}
	return errors
}

func binName(folder string, i int) string {
	return fmt.Sprintf("%s_%02d.png", filepath.Base(folder), i)
}

// A sheet that repacked to identical pixels is the same waste as a .tres that
// did, in a much larger file.
func savePng(img image.Image, folder string, i int) error {
	if err := os.MkdirAll(folder, 0755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}

	_, err := writeIfChanged(filepath.Join(folder, binName(folder, i)), buf.Bytes())
	return err
}

// A folder that shrinks from two bins to one leaves the second sheet behind.
// These are igor's own output, so they go without asking.
func removeOrphanedBins(folder string, bins int) error {
	for i := bins; i < bins+maxLayoutBins; i++ {
		p := filepath.Join(folder, binName(folder, i))
		if _, err := os.Stat(p); err != nil {
			// Bin numbering has no gaps, so the first miss is the end.
			return nil
		}
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	return nil
}

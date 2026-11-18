package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func imgRectFor(r rect) image.Rectangle {
	return image.Rect(r.x, r.y, r.x+r.w, r.y+r.h)
}

func renderBins(bins []spriteBin, imgs []imageFile, trgFolder string, wrote int) []error {

	errors := []error{}
	if len(imgs) == 0 {
		return append(errors, fmt.Errorf("Sprite list was empty, aborting write."))
	}

	for bi, bin := range bins {
		// An interior bin can empty out while later ones stay occupied. Its
		// index has to survive, or every sheet after it is renumbered, but a
		// sheet nothing points at should not sit in the output tree.
		if len(bin.rects) == 0 {
			// Only if igor recorded writing this index. An adopted layout can
			// leave a sparse set of empty bins, and a PNG that happens to sit
			// at one of those names is not igor's to remove.
			if bi < wrote {
				sesh.addStaleSheet(filepath.Join(trgFolder, binName(trgFolder, bi)))
			}
			continue
		}

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

	if err := removeOrphanedBins(trgFolder, len(bins), wrote); err != nil {
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
// These are igor's own output, so they go without asking -- but only the ones
// it recorded writing last time, so a user PNG that happens to be named like a
// sheet is never swept up. Scanned rather than counted upward, because an
// emptied interior bin keeps its index but loses its sheet, so the numbering
// can have gaps.
func removeOrphanedBins(folder string, bins, wrote int) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	prefix := filepath.Base(folder) + "_"
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) {
			continue
		}
		m := binSuffixRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < bins || n >= wrote {
			continue
		}
		sesh.addStaleSheet(filepath.Join(folder, name))
	}
	return nil
}

// Drops the sheets packing superseded. Deferred to the end of a run that
// wrote everything it meant to, so an abort partway cannot leave a .tres
// pointing at a sheet that no longer exists.
//
// A sheet that will not delete is cosmetic: nothing references it, and the
// next rebuild of its folder finds it again. Failing the run over one would
// cost the build cache, and with it the layout, which is a far worse trade.
// So every path is tried and failures are reported rather than raised.
func removeStaleSheets() {
	for _, p := range sesh.takeStaleSheets() {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			warnLate(fmt.Sprintf("could not remove the superseded sheet %s: %s", p, err))
		}
	}
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type writingCompleteMsg struct{}

func writeResourcesCmd(work []workPiece) tea.Cmd {
	return func() tea.Msg {
		sem := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup

		for _, w := range work {
			switch v := w.(type) {
			case workPack:
				wg.Add(1)
				sem <- struct{}{}
				go func(wp workPack) {
					defer wg.Done()
					defer func() { <-sem }()
					writeTres(wp)
				}(v)
			case workSlice:
				// Stub: slice .tres writing will be handled differently
			}
		}

		wg.Wait()
		return writingCompleteMsg{}
	}
}

func writeTres(wp workPack) {
	if len(wp.files) == 0 {
		return
	}

	trgFolder := wp.files[0].targetFolderPath()
	folderName := filepath.Base(trgFolder)

	for bi, bin := range wp.bins {
		// Reconstruct the spritesheet PNG filename (same logic as savePng)
		add := ""
		if len(wp.bins) > 0 {
			add = fmt.Sprintf("_%02d", bi)
		}
		pngName := folderName + add + ".png"

		// Build the res:// path by stripping prj.Destination
		relPath, err := filepath.Rel(prj.Destination, filepath.Join(trgFolder, pngName))
		if err != nil {
			prg.Send(toException(err, nil))
			continue
		}
		resPath := prj.resPath(relPath)

		for _, sr := range bin.rects {
			img := wp.files[sr.i]
			spriteName := strings.TrimSuffix(img.filename, filepath.Ext(img.filename))
			tresPath := filepath.Join(trgFolder, spriteName+".tres")

			content := buildTres(resPath, sr, img)
			if err := os.WriteFile(tresPath, []byte(content), 0644); err != nil {
				prg.Send(toException(err, &img))
			}
		}
	}
}

func buildTres(pngResPath string, sr spriteRect, img imageFile) string {
	var b strings.Builder

	b.WriteString("[gd_resource type=\"AtlasTexture\" load_steps=2 format=3]\n\n")
	b.WriteString(fmt.Sprintf("[ext_resource type=\"Texture2D\" path=\"%s\" id=\"1\"]\n\n", pngResPath))
	b.WriteString("[resource]\n")
	b.WriteString("atlas = ExtResource(\"1\")\n")
	b.WriteString(fmt.Sprintf("region = Rect2(%d, %d, %d, %d)\n", sr.x, sr.y, sr.w, sr.h))

	// Only include margin if there's actual padding.
	// Godot's margin is Rect2(left_offset, top_offset, total_width_expansion, total_height_expansion)
	mL := img.trim.x
	mT := img.trim.y
	mW := img.w - img.trim.w // total horizontal pixels trimmed
	mH := img.h - img.trim.h // total vertical pixels trimmed
	if mL != 0 || mT != 0 || mW != 0 || mH != 0 {
		b.WriteString(fmt.Sprintf("margin = Rect2(%d, %d, %d, %d)\n", mL, mT, mW, mH))
	}

	return b.String()
}

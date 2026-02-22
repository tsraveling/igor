package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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

			if sesh.NewOnly {
				if _, err := os.Stat(tresPath); err == nil {
					sesh.Skipped.Add(1)
					continue
				}
			}

			sesh.Written.Add(1)
			content := buildTres(resPath, sr, img)
			if err := os.WriteFile(tresPath, []byte(content), 0644); err != nil {
				prg.Send(toException(err, &img))
			}
		}
	}
}

func generateRandomID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func writeSpriteFrames(charName string, parentPath string, packs []workPack) {
	// Sort packs by animation name for deterministic output
	sort.Slice(packs, func(i, j int) bool {
		return packs[i].f.name < packs[j].f.name
	})

	type frameRef struct {
		resPath string
		id      string
		anim    string
	}

	var frames []frameRef
	counter := 1

	for _, wp := range packs {
		// Sort files by filename for correct frame ordering
		files := make([]imageFile, len(wp.files))
		copy(files, wp.files)
		sort.Slice(files, func(i, j int) bool {
			return files[i].filename < files[j].filename
		})

		animName := wp.f.name

		for _, img := range files {
			spriteName := strings.TrimSuffix(img.filename, filepath.Ext(img.filename))
			tresRelPath := filepath.Join(img.path, spriteName+".tres")
			resPath := prj.resPath(tresRelPath)
			id := fmt.Sprintf("%d_%s", counter, generateRandomID(5))
			frames = append(frames, frameRef{resPath: resPath, id: id, anim: animName})
			counter++
		}
	}

	var b strings.Builder

	// Header
	b.WriteString("[gd_resource type=\"SpriteFrames\" format=3]\n\n")

	// ext_resource entries
	for _, f := range frames {
		b.WriteString(fmt.Sprintf("[ext_resource type=\"Texture2D\" path=\"%s\" id=\"%s\"]\n", f.resPath, f.id))
	}

	b.WriteString("\n[resource]\n")
	b.WriteString("animations = [")

	// Group frames by animation, preserving order
	animOrder := []string{}
	animFrames := map[string][]frameRef{}
	for _, f := range frames {
		if _, exists := animFrames[f.anim]; !exists {
			animOrder = append(animOrder, f.anim)
		}
		animFrames[f.anim] = append(animFrames[f.anim], f)
	}

	for ai, animName := range animOrder {
		if ai > 0 {
			b.WriteString(", ")
		}
		b.WriteString("{\n")
		b.WriteString("\"frames\": [")

		aFrames := animFrames[animName]
		for fi, f := range aFrames {
			if fi > 0 {
				b.WriteString(", ")
			}
			b.WriteString("{\n")
			b.WriteString("\"duration\": 1.0,\n")
			b.WriteString(fmt.Sprintf("\"texture\": ExtResource(\"%s\")\n", f.id))
			b.WriteString("}")
		}

		b.WriteString("],\n")
		b.WriteString("\"loop\": true,\n")
		b.WriteString(fmt.Sprintf("\"name\": &\"%s\",\n", animName))
		b.WriteString("\"speed\": 5.0\n")
		b.WriteString("}")
	}

	b.WriteString("]\n")

	// Write the file
	outPath := filepath.Join(prj.Destination, parentPath, charName+"_frames.tres")
	if sesh.NewOnly {
		if _, err := os.Stat(outPath); err == nil {
			sesh.Skipped.Add(1)
			return
		}
	}
	sesh.Written.Add(1)
	if err := os.WriteFile(outPath, []byte(b.String()), 0644); err != nil {
		prg.Send(toException(err, nil))
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

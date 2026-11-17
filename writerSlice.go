package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

func writeSliceTscn(ws workSlice) {
	if len(ws.slices) == 0 {
		return
	}

	basename := ws.file.baseName()
	outDir := filepath.Join(prj.Destination, ws.file.path, basename)
	tscnPath := filepath.Join(outDir, basename+".tscn")
	trim := ws.file.trim

	old := readGodotFile(tscnPath)

	resPaths := make([]string, len(ws.slices))
	for i, s := range ws.slices {
		relPath, err := filepath.Rel(prj.Destination, s.path)
		if err != nil {
			prg.Send(toException(err, nil))
			return
		}
		resPaths[i] = prj.resPath(relPath)
	}

	alloc := newIDAllocator(old)
	ids := make([]string, len(ws.slices))
	for i, p := range resPaths {
		ids[i] = alloc.sceneID(p)
	}

	var b strings.Builder

	// Header: load_steps = ext_resources + 1
	b.WriteString(carryHeader(old, "gd_scene", "",
		fmt.Sprintf("[gd_scene load_steps=%d format=3]", len(ws.slices)+1), len(ws.slices)+1))
	b.WriteString("\n\n")

	// ext_resource entries for each slice texture
	for i, p := range resPaths {
		b.WriteString(extResourceLine("Texture2D", p, ids[i], old))
	}

	// Root Node2D
	b.WriteString(fmt.Sprintf("\n[node name=\"%s\" type=\"Node2D\"]\n", basename))

	// Child Sprite2D for each slice, positioned to reassemble the original
	for i, s := range ws.slices {
		id := ids[i]
		sliceName := fmt.Sprintf("%s_x%02d", basename, i+1)
		posX := s.x - trim.x
		posY := s.y - trim.y

		b.WriteString(fmt.Sprintf("\n[node name=\"%s\" type=\"Sprite2D\" parent=\".\"]\n", sliceName))
		if posX != 0 || posY != 0 {
			b.WriteString(fmt.Sprintf("position = Vector2(%d, %d)\n", posX, posY))
		}
		b.WriteString("centered = false\n")
		b.WriteString(fmt.Sprintf("texture = ExtResource(\"%s\")\n", id))
	}

	if _, err := writeIfChanged(tscnPath, []byte(b.String())); err != nil {
		prg.Send(toException(err, &ws.file))
	}
}

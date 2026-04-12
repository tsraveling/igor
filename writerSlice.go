package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeSliceTscn(ws workSlice) {
	if len(ws.slices) == 0 {
		return
	}

	basename := strings.TrimSuffix(ws.file.filename, filepath.Ext(ws.file.filename))
	outDir := filepath.Join(prj.Destination, ws.file.path, basename)
	tscnPath := filepath.Join(outDir, basename+".tscn")
	trim := ws.file.trim

	var b strings.Builder

	// Header: load_steps = ext_resources + 1
	b.WriteString(fmt.Sprintf("[gd_scene load_steps=%d format=3]\n\n", len(ws.slices)+1))

	// ext_resource entries for each slice texture
	for i, s := range ws.slices {
		id := fmt.Sprintf("%d", i+1)
		relPath, err := filepath.Rel(prj.Destination, s.path)
		if err != nil {
			prg.Send(toException(err, nil))
			return
		}
		resPath := prj.resPath(relPath)
		b.WriteString(fmt.Sprintf("[ext_resource type=\"Texture2D\" path=\"%s\" id=\"%s\"]\n", resPath, id))
	}

	// Root Node2D
	b.WriteString(fmt.Sprintf("\n[node name=\"%s\" type=\"Node2D\"]\n", basename))

	// Child Sprite2D for each slice, positioned to reassemble the original
	for i, s := range ws.slices {
		id := fmt.Sprintf("%d", i+1)
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

	if err := os.WriteFile(tscnPath, []byte(b.String()), 0644); err != nil {
		prg.Send(toException(err, &ws.file))
	}
}

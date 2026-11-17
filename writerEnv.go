package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type envLayer struct {
	filename string
	resPath  string
	isScene  bool
	posX     int
	posY     int
}

func writeEnvTscn(folderPath string, packs []workPack, slices []workSlice) {
	folderName := filepath.Base(folderPath)

	var layers []envLayer

	// Add packed sprites (small images → .tres referencing spritesheet)
	for _, wp := range packs {
		for _, img := range wp.files {
			spriteName := img.baseName()
			tresRelPath := filepath.Join(img.path, spriteName+".tres")
			resPath := prj.resPath(tresRelPath)
			layers = append(layers, envLayer{
				filename: spriteName,
				resPath:  resPath,
				isScene:  false,
				posX:     0,
				posY:     0,
			})
		}
	}

	// Add sliced sprites (large images → sub .tscn scenes)
	for _, ws := range slices {
		basename := ws.file.baseName()
		tscnRelPath := filepath.Join(ws.file.path, basename, basename+".tscn")
		resPath := prj.resPath(tscnRelPath)
		layers = append(layers, envLayer{
			filename: basename,
			resPath:  resPath,
			isScene:  true,
			posX:     ws.file.trim.x,
			posY:     ws.file.trim.y,
		})
	}

	if len(layers) == 0 {
		return
	}

	// Sort alphabetically by layer name. Pieces of a divided image carry
	// reading-order suffixes, so they stay next to each other and in order.
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].filename < layers[j].filename
	})

	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("[gd_scene load_steps=%d format=3]\n\n", len(layers)+1))

	// ext_resource entries
	for i, l := range layers {
		id := fmt.Sprintf("%d", i+1)
		if l.isScene {
			b.WriteString(fmt.Sprintf("[ext_resource type=\"PackedScene\" path=\"%s\" id=\"%s\"]\n", l.resPath, id))
		} else {
			b.WriteString(fmt.Sprintf("[ext_resource type=\"Texture2D\" path=\"%s\" id=\"%s\"]\n", l.resPath, id))
		}
	}

	// Root node
	b.WriteString(fmt.Sprintf("\n[node name=\"%s\" type=\"Node2D\"]\n", folderName))

	// Child nodes, layered in alphabetical order
	for i, l := range layers {
		id := fmt.Sprintf("%d", i+1)
		nodeName := l.filename

		if l.isScene {
			b.WriteString(fmt.Sprintf("\n[node name=\"%s\" parent=\".\" instance=ExtResource(\"%s\")]\n", nodeName, id))
			if l.posX != 0 || l.posY != 0 {
				b.WriteString(fmt.Sprintf("position = Vector2(%d, %d)\n", l.posX, l.posY))
			}
		} else {
			b.WriteString(fmt.Sprintf("\n[node name=\"%s\" type=\"Sprite2D\" parent=\".\"]\n", nodeName))
			if l.posX != 0 || l.posY != 0 {
				b.WriteString(fmt.Sprintf("position = Vector2(%d, %d)\n", l.posX, l.posY))
			}
			b.WriteString("centered = false\n")
			b.WriteString(fmt.Sprintf("texture = ExtResource(\"%s\")\n", id))
		}
	}

	// Write the file
	outPath := filepath.Join(prj.Destination, folderPath, folderName+".tscn")
	if err := os.WriteFile(outPath, []byte(b.String()), 0644); err != nil {
		prg.Send(toException(err, nil))
	}
}

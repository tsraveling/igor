package main

import (
	"fmt"
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
		// The same naming writeTres used, or the scene references .tres paths
		// that were never written.
		sortedIndex := alphaIndexes(wp.files, wp.f.config.renameLayers)
		charName := filepath.Base(filepath.Dir(wp.f.path))

		for i, img := range wp.files {
			spriteName := spriteName(wp.f.config, img, wp.f.name, charName, sortedIndex[i])
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

	outPath := filepath.Join(prj.Destination, folderPath, folderName+".tscn")
	old := readGodotFile(outPath)

	// Ids are kept per res:// path rather than by position, so inserting a
	// layer no longer renumbers every id after it.
	alloc := newIDAllocator(old)
	ids := make([]string, len(layers))
	for i, l := range layers {
		ids[i] = alloc.sceneID(l.resPath)
	}

	var b strings.Builder

	// Header
	b.WriteString(carryHeader(old, "gd_scene", "",
		fmt.Sprintf("[gd_scene load_steps=%d format=3]", len(layers)+1), len(layers)+1))
	b.WriteString("\n\n")

	// ext_resource entries
	for i, l := range layers {
		typ := "Texture2D"
		if l.isScene {
			typ = "PackedScene"
		}
		b.WriteString(extResourceLine(typ, l.resPath, ids[i], old))
	}

	// Root node
	b.WriteString(fmt.Sprintf("\n[node name=\"%s\" type=\"Node2D\"]\n", folderName))

	// Child nodes, layered in alphabetical order
	for i, l := range layers {
		id := ids[i]
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

	if _, err := writeIfChanged(outPath, []byte(b.String())); err != nil {
		prg.Send(toException(err, nil))
	}
}

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
	sortedIndex := alphaIndexes(wp.files, wp.f.config.renameLayers)
	charName := filepath.Base(filepath.Dir(wp.f.path))

	for bi, bin := range wp.bins {
		// Build the res:// path by stripping prj.Destination
		relPath, err := filepath.Rel(prj.Destination, filepath.Join(trgFolder, binName(trgFolder, bi)))
		if err != nil {
			prg.Send(toException(err, nil))
			continue
		}
		resPath := prj.resPath(relPath)

		for _, sr := range bin.rects {
			img := wp.files[sr.i]
			sName := spriteName(wp.f.config, img, wp.f.name, charName, sortedIndex[sr.i])
			tresPath := filepath.Join(trgFolder, sName+".tres")

			// The file on disk is the truth about identity: reading it back
			// survives a deleted manifest and a touch in the Godot editor.
			old := readGodotFile(tresPath)
			content := buildTres(resPath, sr, img, wp.f.config.typ != FolderTypeItem, old)
			if _, err := writeIfChanged(tresPath, []byte(content)); err != nil {
				prg.Send(toException(err, &img))
			}
		}
	}

	noteRenames(wp, sortedIndex, charName)
}

// renameLayers names by alphabetical position, so inserting a frame into the
// middle of an animation renames every later one, which means new resources
// and broken references in any scene pointing at them. It cannot be avoided
// without a breaking change to naming, so it fails noisily instead.
func noteRenames(wp workPack, sortedIndex map[int]int, charName string) {
	if !wp.f.config.renameLayers {
		return
	}

	entry, ok := priorManifest.Folders[wp.f.path]
	if !ok || len(entry.Files) == 0 {
		return
	}

	files := append([]fileRecord(nil), entry.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })

	was := map[slotKey]string{}
	n := 0
	for _, fr := range files {
		src := strings.TrimSuffix(fr.File, filepath.Ext(fr.File))
		for _, p := range fr.Pieces {
			n++
			was[slotKey{file: fr.File, outName: p.Name}] =
				spriteNameFor(wp.f.config, src, p.Name, wp.f.name, charName, n)
		}
	}

	for i, img := range wp.files {
		before, ok := was[keyOf(img)]
		if !ok {
			continue
		}
		if now := spriteName(wp.f.config, img, wp.f.name, charName, sortedIndex[i]); now != before {
			sesh.addRename(wp.f.path, before, now)
		}
	}
}

// A .tres with no sprite behind it any more: a deleted frame, or one left
// stranded by a rename. Driven off the finished manifest rather than off this
// run's work queue, so an orphan in a folder that was skipped is still found
// and "run with --clean" actually removes it. Reported always, removed only
// when asked, since igor cannot see the scenes that may still reference it.
func scanOrphans(mf manifest) {
	for folderPath, entry := range mf.Folders {
		if len(entry.Files) == 0 {
			continue
		}

		dir := filepath.Join(prj.Destination, folderPath)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		expected := map[string]bool{}
		for _, name := range manifestSpriteNames(folderPath, getFolderConfig(folderPath), entry) {
			expected[name+".tres"] = true
		}

		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".tres") || expected[name] {
				continue
			}
			// A character's frames resource lives beside its animations and is
			// not one of their sprites.
			if strings.HasSuffix(name, "_frames.tres") {
				continue
			}
			sesh.addOrphan(filepath.Join(dir, name))
		}
	}
}

func spriteName(cfg folderConfig, img imageFile, folderName string, charName string, alphaIndex int) string {
	return spriteNameFor(cfg, strings.TrimSuffix(img.filename, filepath.Ext(img.filename)),
		img.outName, folderName, charName, alphaIndex)
}

// Split out so a name can be derived from a manifest record, with no imageFile
// and no PNG to re-read.
func spriteNameFor(cfg folderConfig, srcBase, outName, folderName, charName string, alphaIndex int) string {
	// A divided piece already carries its reading-order name.
	if outName != "" {
		return outName
	}
	if !cfg.renameLayers {
		return srcBase
	}

	prefix := folderName
	if cfg.includeCharName {
		prefix = charName + "_" + prefix
	}
	return fmt.Sprintf("%s_%02d", prefix, alphaIndex)
}

func generateRandomID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// Built from the manifest rather than the work queue, so an animation that was
// not rebuilt this run still appears, and rebuilding one animation no longer
// drags its siblings along.
func writeSpriteFrames(charName string, parentPath string, mf manifest) {
	type frameRef struct {
		resPath string
		id      string
		anim    string
	}

	animPaths := []string{}
	for p, entry := range mf.Folders {
		if filepath.Dir(p) != parentPath || len(entry.Files) == 0 {
			continue
		}
		if getFolderConfig(p).typ != FolderTypeCharacter {
			continue
		}
		animPaths = append(animPaths, p)
	}
	sort.Strings(animPaths)

	outPath := filepath.Join(prj.Destination, parentPath, charName+"_frames.tres")

	// Every animation under this character is gone, so the resource describing
	// them is igor's own leftover.
	if len(animPaths) == 0 {
		os.Remove(outPath)
		return
	}

	old := readGodotFile(outPath)
	ids := newIDAllocator(old)

	var frames []frameRef
	for _, p := range animPaths {
		cfg := getFolderConfig(p)
		animName := filepath.Base(p)
		for _, name := range manifestSpriteNames(p, cfg, mf.Folders[p]) {
			resPath := prj.resPath(filepath.Join(p, name+".tres"))
			frames = append(frames, frameRef{resPath: resPath, id: ids.resourceID(resPath), anim: animName})
		}
	}

	var b strings.Builder

	b.WriteString(carryHeader(old, "gd_resource", "SpriteFrames",
		"[gd_resource type=\"SpriteFrames\" format=3]", len(frames)+1))
	b.WriteString("\n\n")

	for _, f := range frames {
		b.WriteString(extResourceLine("Texture2D", f.resPath, f.id, old))
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

	if _, err := writeIfChanged(outPath, []byte(b.String())); err != nil {
		prg.Send(toException(err, nil))
	}
}

func buildTres(pngResPath string, sr spriteRect, img imageFile, withMargin bool, old *godotFile) string {
	var b strings.Builder

	b.WriteString(carryHeader(old, "gd_resource", "AtlasTexture",
		"[gd_resource type=\"AtlasTexture\" load_steps=2 format=3]", 2))
	b.WriteString("\n\n")
	b.WriteString(extResourceLine("Texture2D", pngResPath, "1", old))
	b.WriteString("\n[resource]\n")
	b.WriteString("atlas = ExtResource(\"1\")\n")
	b.WriteString(fmt.Sprintf("region = Rect2(%d, %d, %d, %d)\n", sr.x, sr.y, sr.w, sr.h))

	// Item textures stand alone rather than reassembling into a scene, so they
	// keep no record of where they sat on the source canvas.
	if !withMargin {
		return b.String()
	}

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

package main

import (
	"errors"
	"fmt"
	"io/fs"
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

	names := make([]string, len(wp.files))
	for i, img := range wp.files {
		names[i] = spriteName(wp.f.config, img, wp.f.name, charName, sortedIndex[i])
	}

	identity := identityPolicyFor(wp, charName, trgFolder, names)
	switch {
	case identity.reassigning():
		warnLate(fmt.Sprintf("%s: no build cache and the sprite names have changed, so uids here are being reassigned rather than guessed", wp.f.path))
	case identity.trusting():
		warnLate(fmt.Sprintf("%s: no build cache, so uids here are kept on the assumption that these names still mean the same frames. Commit %s to make that certain", wp.f.path, manifestFilename))
	}

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
			sName := names[sr.i]
			tresPath := filepath.Join(trgFolder, sName+".tres")

			// The file on disk is the truth about identity, but only where the
			// name can be attributed to the same source that wrote it.
			var old *godotFile
			if identity.carries(sName, keyOf(img)) {
				old = readGodotFile(tresPath)
			}

			// Either the name could not be attributed, or there was no file to
			// read. Both leave the sprite with no uid, which the resources
			// referencing it must not quietly put back.
			if old == nil {
				sesh.markIdentityReset(prj.resPath(filepath.Join(wp.f.path, sName+".tres")))
			}

			content := buildTres(resPath, sr, img, wp.f.config.typ != FolderTypeItem, old)
			if _, err := writeIfChanged(tresPath, []byte(content)); err != nil {
				prg.Send(toException(err, &img))
			}
		}
	}

	noteRenames(wp, sortedIndex, charName)
}

// Whether the uid sitting at an output name may be re-attached to the sprite
// about to take that name. Deliberately an allowlist: a uid is carried only
// where the name is attributable, because a uid on the wrong art is worse than
// no uid at all, and the file on disk cannot say which source wrote it.
type identityPolicy struct {
	// Names derived from position rather than from the source filename, which
	// are the only ones that can come to mean a different image.
	positional bool

	// What each name meant on the previous build. Nil when there is no record
	// to compare against, which is the case on the first run after a format
	// bump and on a clone with no build cache.
	owners map[string]slotKey

	// Standing in for that record: the names on disk are exactly the names
	// about to be written, so none was added or removed.
	sameNames bool

	// How many pieces each divided source image was cut into, last build and
	// this one. A divided piece's name is its reading-order position, so these
	// counts are the only thing that says whether a name still means the same
	// blob; its slotKey carries that same positional name, which would make
	// the owner comparison below compare a key to itself.
	priorPieces map[string]int
	nowPieces   map[string]int

	// Whether there was any previous output at all. A first build has no
	// identity to carry and is assuming nothing.
	rebuild bool
}

func identityPolicyFor(wp workPack, charName, trgFolder string, names []string) identityPolicy {
	p := identityPolicy{
		positional: wp.f.config.renameLayers,
		owners:     priorOwners(wp, charName),
	}

	// A divided image's pieces are numbered in reading order, so they shift
	// for the same reason renamed layers do.
	p.nowPieces = map[string]int{}
	for _, img := range wp.files {
		if img.outName == "" {
			continue
		}
		p.positional = true
		p.nowPieces[img.filename]++
	}

	p.priorPieces = map[string]int{}
	for _, fr := range priorManifest.Folders[wp.f.path].Files {
		for _, piece := range fr.Pieces {
			if piece.Name != "" {
				p.priorPieces[fr.File]++
			}
		}
	}

	if p.positional && p.owners == nil {
		p.sameNames, p.rebuild = onDiskNamesMatch(trgFolder, names)
	}
	return p
}

func (p identityPolicy) carries(name string, key slotKey) bool {
	// The name is the source's own, so it cannot come to mean another image.
	// Re-adding a deleted file legitimately reclaims the identity of its path,
	// which is how Godot treats it too.
	if !p.positional {
		return true
	}

	if p.owners == nil {
		return p.sameNames
	}

	// Inserting a blob renumbers every later piece of the same image, so the
	// piece that holds a name is only the same piece if the count has not
	// moved. Comparing owners here would always agree: both sides are keyed
	// by the positional name itself.
	if key.outName != "" {
		was, known := p.priorPieces[key.file]
		return known && was == p.nowPieces[key.file]
	}

	// A name the previous build never wrote has no identity to inherit. The
	// file sitting there is an orphan from some older build, and its uid
	// belongs to art that is no longer at this name.
	owner, known := p.owners[name]
	return known && owner == key
}

// True when uids are being dropped wholesale for want of anything to attribute
// them by, which is worth saying out loud.
func (p identityPolicy) reassigning() bool {
	return p.positional && p.owners == nil && !p.sameNames
}

// True when positional names are being taken at face value because there is no
// build record to check them against. Sound unless a frame was removed and
// another added in the same run, which leaves the set of names identical while
// shifting what those in between mean -- the one case nothing on disk can
// reveal, and the reason the README asks for the cache to be committed.
func (p identityPolicy) trusting() bool {
	return p.positional && p.owners == nil && p.sameNames && p.rebuild
}

// Whether the .tres already in a folder are exactly the ones about to be
// written. Weaker than a build record -- a delete and an add in one run leave
// the set unchanged while shifting what the names in between mean -- which is
// why the README asks for the build cache to be committed.
// Reports whether the names match, and whether there was any output to match
// against in the first place.
func onDiskNamesMatch(trgFolder string, names []string) (match, rebuild bool) {
	entries, err := os.ReadDir(trgFolder)
	if err != nil {
		return false, false
	}

	// Only igor's own sprites say anything about what the names mean. Counting
	// a resource someone put here by hand would read as a changed name set and
	// cost the folder every uid it has.
	found := map[string]bool{}
	folderName := filepath.Base(trgFolder)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tres") {
			continue
		}
		if _, ours := igorsOwnSprite(filepath.Join(trgFolder, e.Name()), folderName); !ours {
			continue
		}
		found[e.Name()] = true
	}

	// Nothing on disk is a first build, where there is no identity to lose and
	// nothing to carry either way.
	if len(found) == 0 {
		return true, false
	}
	if len(found) != len(names) {
		return false, true
	}
	for _, n := range names {
		if !found[n+".tres"] {
			return false, true
		}
	}
	return true, true
}

// renameLayers names by alphabetical position, so inserting a frame into the
// middle of an animation renames every later one, which means new resources
// and broken references in any scene pointing at them. It cannot be avoided
// without a breaking change to naming, so it fails noisily instead.
func noteRenames(wp workPack, sortedIndex map[int]int, charName string) {
	was := priorNames(wp, charName)
	if was == nil {
		return
	}

	// A divided image renumbers its pieces in reading order, so inserting a
	// blob shifts every later piece onto the next name. There is no stable
	// per-piece identity to diff, so the count is what gets reported.
	prior := map[string]int{}
	for _, fr := range priorManifest.Folders[wp.f.path].Files {
		for _, piece := range fr.Pieces {
			if piece.Name != "" {
				prior[fr.File]++
			}
		}
	}
	now := map[string]int{}
	for _, img := range wp.files {
		if img.outName != "" {
			now[img.filename]++
		}
	}
	for file, n := range now {
		if was, known := prior[file]; known && was != n {
			sesh.addRename(wp.f.path, fmt.Sprintf("%s cut into %d pieces", file, was),
				fmt.Sprintf("%d, renumbering them all", n))
		}
	}

	if !wp.f.config.renameLayers {
		return
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

// The output name each source held on the previous build, derived from the
// manifest. Nil when there is no record to compare against.
func priorNames(wp workPack, charName string) map[slotKey]string {
	entry, ok := priorManifest.Folders[wp.f.path]
	if !ok || len(entry.Files) == 0 {
		return nil
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
	return was
}

// Which source owned each output name last build, so a name that has changed
// hands can be told apart from one that merely changed pixels.
func priorOwners(wp workPack, charName string) map[string]slotKey {
	was := priorNames(wp, charName)
	if was == nil {
		// Distinct from a record that exists and is empty: callers treat nil
		// as "nothing to compare against" and fall back to the disk check.
		return nil
	}

	out := map[string]slotKey{}
	for key, name := range was {
		out[name] = key
	}
	return out
}

// A .tres igor wrote on an earlier build and no longer writes: a deleted
// frame, or one left stranded by a rename. Driven off the record the manifest
// keeps rather than off this run's work queue, so an orphan in a folder that
// was skipped is still found and "run with --clean" actually removes it.
//
// The record is also what makes deletion safe. Deciding from a file's shape
// whether igor wrote it cannot distinguish a stranded sprite from a
// hand-authored AtlasTexture cropping the same sheet, and --clean --force
// deletes without asking.
func scanOrphans(mf manifest) {
	for folderPath, entry := range mf.Folders {
		if len(entry.Orphans) == 0 {
			continue
		}

		dir := filepath.Join(prj.Destination, folderPath)

		// Sheets the surviving sprites still draw from, which no orphan can
		// make reclaimable.
		live := map[string]bool{}
		for _, fr := range entry.Files {
			for _, p := range fr.Pieces {
				live[binName(dir, p.Bin)] = true
			}
		}

		for _, name := range entry.Orphans {
			path := filepath.Join(dir, name+".tres")
			if _, err := os.Stat(path); err != nil {
				continue
			}
			sesh.addOrphan(path)

			// A sheet no surviving sprite draws from is kept alive only by
			// this orphan, so it waits here rather than being reclaimed out
			// from under it.
			if sheet := sheetOf(path); sheet != "" && !live[sheet] {
				sesh.orphanSheet(filepath.Join(dir, sheet))
			}
		}
	}
}

// The spritesheet a sprite draws from, used only to decide which sheet an
// orphan is holding open.
func sheetOf(path string) string {
	g := readGodotFile(path)
	if g == nil || len(g.paths) != 1 {
		return ""
	}
	return resBase(g.paths[0])
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

	type animation struct {
		path  string
		names []string
	}

	animations := []animation{}
	known := 0
	for p, entry := range mf.Folders {
		if filepath.Dir(p) != parentPath || len(entry.Files) == 0 {
			continue
		}
		if getFolderConfig(p).typ != FolderTypeCharacter {
			continue
		}
		known++

		// The manifest is a cache, and a stale entry must degrade to a missing
		// animation rather than to a resource Godot cannot load.
		names := manifestSpriteNames(p, getFolderConfig(p), entry)
		missing, err := firstMissingTres(p, names)
		if err != nil {
			// An unreadable folder is a fault, not a cache miss. Dropping the
			// animation over it would quietly shrink the character.
			prg.Send(toException(err, nil))
		} else if missing != "" {
			warnLate(fmt.Sprintf("%s: left out of %s_frames.tres, %s is not on disk", p, charName, missing))
			continue
		}
		animations = append(animations, animation{path: p, names: names})
	}
	sort.Slice(animations, func(i, j int) bool { return animations[i].path < animations[j].path })

	outPath := filepath.Join(prj.Destination, parentPath, charName+"_frames.tres")

	if len(animations) == 0 {
		// Every animation under this character is gone, so the resource
		// describing them is igor's own leftover. If they are merely missing
		// their output, the existing resource is left alone rather than
		// deleted over a fault that may be temporary.
		if known == 0 {
			os.Remove(outPath)
		}
		return
	}

	old := readGodotFile(outPath)
	ids := newIDAllocator(old)

	var frames []frameRef
	for _, a := range animations {
		animName := filepath.Base(a.path)
		for _, name := range a.names {
			resPath := prj.resPath(filepath.Join(a.path, name+".tres"))
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

// Whether a .tres looks like one igor wrote into this folder: an AtlasTexture
// over a single spritesheet of this folder's own.
//
// This is a guess from shape, and a hand-authored AtlasTexture cropping igor's
// own sheet matches it, so nothing destructive may rest on it. It is used only
// where there is no record to consult, and only to judge whether a folder's
// names can be trusted, where being wrong costs uids and prints a warning.
// Deleting files goes through folderEntry.Orphans instead.
func igorsOwnSprite(path, folderName string) (sheet string, ours bool) {
	g := readGodotFile(path)
	if g == nil || g.kind != "gd_resource" || g.typ != "AtlasTexture" {
		return "", false
	}
	if !g.hasRegion || len(g.paths) != 1 {
		return "", false
	}
	if _, ok := binIndexOf(g.paths[0], folderName); !ok {
		return "", false
	}
	return resBase(g.paths[0]), true
}

// The first sprite the manifest claims a folder has that is not actually
// there, or empty when they are all present.
func firstMissingTres(folderPath string, names []string) (string, error) {
	dir := filepath.Join(prj.Destination, folderPath)
	for _, n := range names {
		_, err := os.Stat(filepath.Join(dir, n+".tres"))
		if err == nil {
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			return n + ".tres", nil
		}
		return "", err
	}
	return "", nil
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

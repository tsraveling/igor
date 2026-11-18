package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A fresh pack has to beat the current one by this much before igor suggests
// --repack. Measured against rendered sheet area, so a folder that packed this
// way from scratch never nags.
const repackGainThreshold = 0.35

// Sanity ceiling on a sheet index recovered from a manifest or a .tres, so a
// corrupt or hand-edited one cannot ask for an absurd number of bins.
const maxLayoutBins = 1024

// One placed piece. Output names move under renameLayers, so the slot a sprite
// holds is keyed by its source filename plus its divided-piece name.
type slotKey struct {
	file    string
	outName string
}

func keyOf(f imageFile) slotKey {
	return slotKey{file: f.filename, outName: f.outName}
}

// Placement for one folder. Honours the stored layout where it can, so a
// changed PNG redraws its own region and leaves every sibling where it sat.
func packFolder(w workPack) ([]spriteBin, bool) {
	if sesh.Repack {
		return maxRects(w.files, AlgoBSSF), false
	}

	slots := storedSlots(w.f.path)
	if slots == nil {
		slots = adoptedSlots(w)
	}
	if slots == nil {
		return maxRects(w.files, AlgoBSSF), false
	}

	bins, ok := repackAround(w.files, slots)
	if !ok {
		return maxRects(w.files, AlgoBSSF), false
	}
	return bins, true
}

func storedSlots(folderPath string) map[slotKey]piecePlacement {
	entry, ok := priorManifest.Folders[folderPath]
	if !ok || len(entry.Files) == 0 {
		return nil
	}

	slots := map[slotKey]piecePlacement{}
	for _, fr := range entry.Files {
		for _, p := range fr.Pieces {
			slots[slotKey{file: fr.File, outName: p.Name}] = p
		}
	}
	if len(slots) == 0 {
		return nil
	}
	return slots
}

var binSuffixRe = regexp.MustCompile(`_(\d+)\.png$`)

// Which sheet a .tres pointed at, from the spritesheet path it carried.
func binIndexOf(resPath, folderName string) (int, bool) {
	base := resBase(resPath)
	if !strings.HasPrefix(base, folderName+"_") {
		return 0, false
	}
	m := binSuffixRe.FindStringSubmatch(base)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 0 || n >= maxLayoutBins {
		return 0, false
	}
	return n, true
}

// res:// paths are always slash-separated, whatever the host filesystem uses.
func resBase(resPath string) string {
	return resPath[strings.LastIndex(resPath, "/")+1:]
}

// Recovers a folder's layout from the .tres files already on disk. This is
// what makes the manifest format bump a near-zero diff, and it doubles as the
// recovery path for a manifest that was deleted or never committed.
func adoptedSlots(w workPack) map[slotKey]piecePlacement {
	if len(w.files) == 0 {
		return nil
	}

	trgFolder := w.files[0].targetFolderPath()
	if _, err := os.Stat(trgFolder); err != nil {
		return nil
	}

	folderName := filepath.Base(trgFolder)
	charName := filepath.Base(filepath.Dir(w.f.path))
	idx := alphaIndexes(w.files, w.f.config.renameLayers)

	slots := map[slotKey]piecePlacement{}
	for i, img := range w.files {
		sName := spriteName(w.f.config, img, w.f.name, charName, idx[i])
		g := readGodotFile(filepath.Join(trgFolder, sName+".tres"))

		// A file with nothing to recover is simply placed fresh, rather than
		// costing the whole folder its layout.
		if g == nil || !g.hasRegion || len(g.paths) != 1 {
			continue
		}
		bin, ok := binIndexOf(g.paths[0], folderName)
		if !ok {
			continue
		}

		slots[keyOf(img)] = piecePlacement{
			Name: img.outName,
			Bin:  bin,
			X:    g.region.x,
			Y:    g.region.y,
			W:    g.region.w,
			H:    g.region.h,
		}
	}

	if len(slots) == 0 {
		return nil
	}
	return slots
}

// Keeps every sprite whose trimmed size still fits the slot it held, rebuilds
// the free space around them, and places the rest into what is left. Content
// decides whether a region is redrawn; only size decides whether it moves.
func repackAround(files []imageFile, slots map[slotKey]piecePlacement) ([]spriteBin, bool) {
	keptBin := make([]int, len(files))
	maxBin := -1

	for i, f := range files {
		keptBin[i] = -1
		p, ok := slots[keyOf(f)]
		if !ok || p.W != f.trim.w || p.H != f.trim.h {
			continue
		}
		// Only a sanity bound. Tying this to the number of files or recovered
		// slots rejects real slots: deleting the sprites on the early sheets
		// leaves the survivors sitting on legitimately high bins, and an
		// unreadable .tres shrinks the recovered set without shrinking the
		// layout. An over-large index now costs nothing anyway, since an empty
		// bin renders no sheet.
		if p.X < 0 || p.Y < 0 || p.Bin < 0 || p.Bin >= maxLayoutBins {
			continue
		}
		if p.X+p.W > prj.SpritesheetSize || p.Y+p.H > prj.SpritesheetSize {
			continue
		}
		keptBin[i] = p.Bin
		if p.Bin > maxBin {
			maxBin = p.Bin
		}
	}

	if maxBin < 0 {
		return nil, false
	}

	bins := make([]spriteBin, maxBin+1)
	for i := range bins {
		bins[i].freeRects = []rect{{w: prj.SpritesheetSize, h: prj.SpritesheetSize}}
	}

	for i, f := range files {
		b := keptBin[i]
		if b < 0 {
			continue
		}
		p := slots[keyOf(f)]
		sprite := spriteRect{rect: rect{x: p.X, y: p.Y, w: p.W, h: p.H}, i: i}

		// An adopted layout can be inconsistent in ways igor's own never is,
		// so overlap costs the folder its layout rather than its pixels.
		for _, other := range bins[b].rects {
			if other.rect.intersects(sprite.rect) {
				return nil, false
			}
		}

		bins[b].rects = append(bins[b].rects, sprite)
		bins[b].freeRects = splitFreeRects(bins[b].freeRects, sprite)
	}

	for i, f := range files {
		if keptBin[i] >= 0 {
			continue
		}
		if f.trim.w > prj.SpritesheetSize || f.trim.h > prj.SpritesheetSize {
			prg.Send(exception{code: errorTooLarge, file: &files[i], msg: fmt.Sprintf(
				"%s has trimmed dimensions %d x %d, which exceeds spritesheet size %d",
				f.filename, f.trim.w, f.trim.h, prj.SpritesheetSize)})
			continue
		}
		bins = placeSprite(bins, i, f.trim.w, f.trim.h, AlgoBSSF)
	}

	return trimTrailingBins(bins), true
}

// An empty bin in the middle keeps its index, because dropping it would
// renumber every sheet after it and rewrite every .tres that pointed at them.
func trimTrailingBins(bins []spriteBin) []spriteBin {
	last := -1
	for i, b := range bins {
		if len(b.rects) > 0 {
			last = i
		}
	}
	return bins[:last+1]
}

// The area each bin actually renders to, which is what a sheet costs in vram.
func binArea(bins []spriteBin) int {
	total := 0
	for _, b := range bins {
		maxX, maxY := 0, 0
		for _, r := range b.rects {
			maxX = max(maxX, r.rect.r())
			maxY = max(maxY, r.rect.b())
		}
		total += sheetDim(maxX) * sheetDim(maxY)
	}
	return total
}

// The power-of-two sheet a bin renders to, halved down from the project size
// while the sprites still fit. The comparison is >=, because content whose
// extent is exactly half a sheet fits that half exactly; > left it on the
// larger size, costing four times the texture memory at every power of two.
func sheetDim(extent int) int {
	d := prj.SpritesheetSize
	for d > 1 && d/2 >= extent {
		d /= 2
	}
	return d
}

// Incremental placement leaves holes, so sheets drift larger over time.
// Repacking only helps where a fresh pack would actually do better, so this
// compares against one rather than guessing from waste alone. Never automatic:
// silent reflow is the surprise this whole feature exists to remove.
func repackHint(w workPack, bins []spriteBin) string {
	fresh := maxRects(measurable(w.files), AlgoBSSF)
	cur, gain := binArea(bins), binArea(fresh)

	// An empty interior bin keeps its index but renders no sheet, so counting
	// the slice would both overstate what is on disk and let a folder skip the
	// threshold on a bin count it does not really have.
	occupied := 0
	for _, b := range bins {
		if len(b.rects) > 0 {
			occupied++
		}
	}

	// Fewer bins can still cost more area, and recommending that would be
	// advice to make things worse.
	if gain >= cur {
		return ""
	}
	if len(fresh) >= occupied && float64(gain) > float64(cur)*(1-repackGainThreshold) {
		return ""
	}

	saved := int(100 * float64(cur-gain) / float64(cur))
	if saved < 1 {
		return ""
	}
	return fmt.Sprintf("%s: --repack would save %d%% of its sheet area (%s now, %s packed fresh)",
		w.f.path, saved, plural(occupied, "bin"), plural(len(fresh), "bin"))
}

// maxRects reports an oversized sprite as it walks the list, and the pack this
// is measuring against has already done that, so a second pass must not hand
// the same file over twice.
func measurable(files []imageFile) []imageFile {
	out := make([]imageFile, 0, len(files))
	for _, f := range files {
		if f.trim.w <= prj.SpritesheetSize && f.trim.h <= prj.SpritesheetSize {
			out = append(out, f)
		}
	}
	return out
}

// 1-based alphabetical position of each file, for renameLayers. A nil map
// reads back zero, which is what the unrenamed path already expects.
func alphaIndexes(files []imageFile, rename bool) map[int]int {
	if !rename {
		return nil
	}

	order := make([]int, len(files))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return files[order[a]].filename < files[order[b]].filename
	})

	out := make(map[int]int, len(files))
	for pos, i := range order {
		out[i] = pos + 1
	}
	return out
}

// Output sprite names for a folder, derived from its manifest record alone.
// Section 5 builds {char}_frames.tres from these rather than from whichever
// folders happened to be rebuilt this run.
func manifestSpriteNames(folderPath string, cfg folderConfig, entry folderEntry) []string {
	folderName := filepath.Base(folderPath)
	charName := filepath.Base(filepath.Dir(folderPath))

	files := append([]fileRecord(nil), entry.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })

	names := []string{}
	n := 0
	for _, fr := range files {
		src := strings.TrimSuffix(fr.File, filepath.Ext(fr.File))
		for _, p := range fr.Pieces {
			n++
			names = append(names, spriteNameFor(cfg, src, p.Name, folderName, charName, n))
		}
	}
	return names
}

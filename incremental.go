package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Digests computed during this run, keyed by folder path. Written once during
// dirty detection, read again when the manifest is saved.
var runDigests = map[string]string{}

// Cheap source scan: folder path -> set of PNG basenames. No file reads, so it
// can run before the TUI starts, where the --clean prompt has to live.
func scanSourceFolders() map[string]map[string]bool {
	found := map[string]map[string]bool{}

	filepath.WalkDir(prj.Source, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".png") {
			return nil
		}

		relPath, err := filepath.Rel(prj.Source, p)
		if err != nil {
			return nil
		}
		dir := filepath.Dir(relPath)
		if found[dir] == nil {
			found[dir] = map[string]bool{}
		}
		found[dir][strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))] = true
		return nil
	})

	return found
}

type stalePath struct {
	path   string // output path to remove
	folder string // source folder it belongs to
	isDir  bool   // whole folder vs a single slice subdirectory
}

// Output that the manifest knows about but the source no longer has: folders
// that vanished, and slice subdirectories whose source PNG is gone.
func findStale(mf manifest, source map[string]map[string]bool) []stalePath {
	stale := []stalePath{}

	for folderPath, entry := range mf.Folders {
		files, stillThere := source[folderPath]

		if !stillThere {
			out := filepath.Join(prj.Destination, folderPath)
			if _, err := os.Stat(out); err == nil {
				stale = append(stale, stalePath{path: out, folder: folderPath, isDir: true})
			}
			continue
		}

		for _, s := range entry.Slices {
			if files[s.Src] {
				continue
			}
			out := filepath.Join(prj.Destination, folderPath, s.Dir)
			if _, err := os.Stat(out); err == nil {
				stale = append(stale, stalePath{path: out, folder: folderPath})
			}
		}
	}

	sort.Slice(stale, func(i, j int) bool { return stale[i].path < stale[j].path })
	return stale
}

// Marks the folder set that has to rebuild, and returns it alongside the image
// count so the trimming progress has a total that matches the filtered work.
func filterDirty(all []folder) (kept []folder, images int, skipped int) {
	for _, f := range all {
		runDigests[f.path] = folderDigest(f)
	}

	priorManifest = loadManifest()

	// A character animation whose source and output are both gone is pruned
	// from the manifest, but nothing would otherwise rewrite the frames
	// resource still referencing it: there is no work for it and no prune.
	for p := range priorManifest.Folders {
		if _, stillSource := runDigests[p]; stillSource {
			continue
		}
		if _, err := os.Stat(filepath.Join(prj.Destination, p)); err == nil {
			continue
		}
		if getFolderConfig(p).typ == FolderTypeCharacter {
			sesh.FramesDirty = append(sesh.FramesDirty, filepath.Dir(p))
		}
	}

	// --repack has nothing to do unless the folders reach the work queue, and
	// a run that skipped everything would silently ignore the flag.
	if sesh.All || sesh.Nuke || sesh.Repack {
		return all, countImages(all), 0
	}

	globals := globalsDigest()

	for _, f := range all {
		entry, known := priorManifest.Folders[f.path]

		// A manifest can outlive the output it describes.
		outputMissing := false
		if _, err := os.Stat(filepath.Join(prj.Destination, f.path)); err != nil {
			outputMissing = true
		}

		if !known || entry.Digest != runDigests[f.path] || priorManifest.Globals != globals ||
			outputMissing || outputsIncomplete(f.path, entry) {
			kept = append(kept, f)
		}
	}

	return kept, countImages(kept), len(all) - len(kept)
}

// The manifest is load-bearing for correctness now, not just for speed:
// {char}_frames.tres is built from it, so a record pointing at a .tres that is
// no longer on disk has to rebuild the folder rather than emit a dangling path.
func outputsIncomplete(folderPath string, entry folderEntry) bool {
	dir := filepath.Join(prj.Destination, folderPath)
	cfg := getFolderConfig(folderPath)

	// Each oversized image reassembles into a scene of its own.
	for _, s := range entry.Slices {
		if _, err := os.Stat(filepath.Join(dir, s.Dir, s.Dir+".tscn")); err != nil {
			return true
		}
	}

	// An env package's layers are reassembled by a scene beside them.
	if cfg.typ == FolderTypeEnv {
		if _, err := os.Stat(filepath.Join(dir, filepath.Base(folderPath)+".tscn")); err != nil {
			return true
		}
	}

	if len(entry.Files) == 0 {
		return false
	}

	for _, name := range manifestSpriteNames(folderPath, cfg, entry) {
		if _, err := os.Stat(filepath.Join(dir, name+".tres")); err != nil {
			return true
		}
	}

	// A .tres is no use without the sheet it points at. Only the bins the
	// layout actually places into are checked, since an emptied interior bin
	// keeps its index but legitimately has no sheet.
	used := map[int]bool{}
	for _, fr := range entry.Files {
		for _, p := range fr.Pieces {
			used[p.Bin] = true
		}
	}
	for bin := range used {
		if _, err := os.Stat(filepath.Join(dir, binName(dir, bin))); err != nil {
			return true
		}
	}

	// A character's frames resource is written from these records too, and
	// nothing else would notice it had gone: with no dirty folder there is no
	// parent to rewrite it for.
	if cfg.typ == FolderTypeCharacter {
		parent := filepath.Dir(folderPath)
		frames := filepath.Join(prj.Destination, parent, filepath.Base(parent)+"_frames.tres")
		if _, err := os.Stat(frames); err != nil {
			return true
		}
	}

	return false
}

func countImages(folders []folder) int {
	n := 0
	for _, f := range folders {
		n += len(f.files)
	}
	return n
}

// Folders that produced at least one exception can't be recorded as clean.
// An exception with no file attached can't be attributed at all, so it spoils
// the whole run.
func buildManifest(built []folder, work []workPiece, exceptions []exception) (manifest, bool) {
	slices := map[string][]sliceEntry{}
	packs := map[string]workPack{}
	for _, w := range work {
		switch v := w.(type) {
		case workSlice:
			src := strings.TrimSuffix(v.file.filename, filepath.Ext(v.file.filename))
			slices[v.f.path] = append(slices[v.f.path], sliceEntry{Dir: v.file.baseName(), Src: src})
		case workPack:
			packs[v.f.path] = v
		}
	}

	failed := map[string]bool{}
	for _, exc := range exceptions {
		if exc.file == nil {
			return manifest{}, false
		}
		failed[exc.file.path] = true
	}

	mf := loadManifest()
	mf.FormatVersion = manifestFormatVersion
	mf.Globals = globalsDigest()

	for _, p := range sesh.Pruned {
		delete(mf.Folders, p)
	}

	// A folder whose source and output are both gone is visited by neither
	// stale detection, which needs the output, nor dirty detection, which
	// needs the source. Left alone it lingers forever and puts dangling paths
	// into {char}_frames.tres.
	for p := range mf.Folders {
		if _, stillSource := runDigests[p]; stillSource {
			continue
		}
		if _, err := os.Stat(filepath.Join(prj.Destination, p)); err == nil {
			continue
		}
		delete(mf.Folders, p)
	}

	for _, f := range built {
		if failed[f.path] {
			// Dropping the entry outright would also drop the record of what
			// igor wrote here, so its superseded sheets and orphans could
			// never be attributed again. Keeping it without a digest leaves
			// the folder dirty, which is the part that matters.
			was := mf.Folders[f.path]
			mf.Folders[f.path] = folderEntry{Bins: was.Bins, Orphans: was.Orphans}
			continue
		}
		entries := slices[f.path]
		sort.Slice(entries, func(i, j int) bool { return entries[i].Dir < entries[j].Dir })

		entry := folderEntry{Digest: runDigests[f.path], Slices: entries}
		if wp, ok := packs[f.path]; ok {
			entry.Bins = len(wp.bins)
			entry.Files = placementRecords(wp)
		}
		entry.Orphans = retiredSprites(f.path, entry)
		mf.Folders[f.path] = entry
	}

	return mf, true
}

// Sprites this folder used to have and no longer writes, plus any from older
// builds that are still sitting there. This is the whole basis on which
// --clean decides a file is igor's to delete, so it is kept as a record rather
// than inferred later from what a file happens to look like.
func retiredSprites(folderPath string, entry folderEntry) []string {
	cfg := getFolderConfig(folderPath)

	current := map[string]bool{}
	for _, n := range manifestSpriteNames(folderPath, cfg, entry) {
		current[n] = true
	}

	was := priorManifest.Folders[folderPath]
	candidates := append(manifestSpriteNames(folderPath, cfg, was), was.Orphans...)

	dir := filepath.Join(prj.Destination, folderPath)
	seen := map[string]bool{}
	retired := []string{}
	for _, n := range candidates {
		if current[n] || seen[n] {
			continue
		}
		// Only while it is actually there; once removed it stops being ours.
		if _, err := os.Stat(filepath.Join(dir, n+".tres")); err != nil {
			continue
		}
		seen[n] = true
		retired = append(retired, n)
	}

	sort.Strings(retired)
	return retired
}

// Where every piece of every packed source file landed, so the next run can
// honour the same slots instead of reflowing the sheet around a single edit.
func placementRecords(wp workPack) []fileRecord {
	placed := make([]piecePlacement, len(wp.files))
	found := make([]bool, len(wp.files))

	for bi, bin := range wp.bins {
		for _, sr := range bin.rects {
			if sr.i < 0 || sr.i >= len(wp.files) {
				continue
			}
			placed[sr.i] = piecePlacement{
				Name: wp.files[sr.i].outName,
				Bin:  bi,
				X:    sr.x, Y: sr.y, W: sr.w, H: sr.h,
			}
			found[sr.i] = true
		}
	}

	byFile := map[string]*fileRecord{}
	names := []string{}
	for i, f := range wp.files {
		if !found[i] {
			continue
		}
		rec, ok := byFile[f.filename]
		if !ok {
			rec = &fileRecord{File: f.filename}
			byFile[f.filename] = rec
			names = append(names, f.filename)
		}
		rec.Pieces = append(rec.Pieces, placed[i])
	}

	sort.Strings(names)
	out := make([]fileRecord, 0, len(names))
	for _, n := range names {
		out = append(out, *byFile[n])
	}
	return out
}

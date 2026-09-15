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

		for _, basename := range entry.Slices {
			if files[basename] {
				continue
			}
			out := filepath.Join(prj.Destination, folderPath, basename)
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

	if sesh.All || sesh.Nuke {
		return all, countImages(all), 0
	}

	mf := loadManifest()
	globals := globalsDigest()

	dirty := map[string]bool{}
	for _, f := range all {
		entry, known := mf.Folders[f.path]

		// A manifest can outlive the output it describes.
		outputMissing := false
		if _, err := os.Stat(filepath.Join(prj.Destination, f.path)); err != nil {
			outputMissing = true
		}

		if !known || entry.Digest != runDigests[f.path] || mf.Globals != globals || outputMissing {
			dirty[f.path] = true
		}
	}

	expandCharSiblings(all, dirty)

	for _, f := range all {
		if dirty[f.path] {
			kept = append(kept, f)
		}
	}
	return kept, countImages(kept), len(all) - len(kept)
}

// Character animations share a {char}_frames.tres written from whichever packs
// are in the work queue, so a rebuilt character folder has to drag its siblings
// along or the others drop out of the resource.
func expandCharSiblings(all []folder, dirty map[string]bool) {
	parents := map[string]bool{}

	for _, f := range all {
		if dirty[f.path] && f.config.typ == FolderTypeCharacter {
			parents[filepath.Dir(f.path)] = true
		}
	}
	for _, pruned := range sesh.Pruned {
		if getFolderConfig(pruned).typ == FolderTypeCharacter {
			parents[filepath.Dir(pruned)] = true
		}
	}

	for _, f := range all {
		if f.config.typ == FolderTypeCharacter && parents[filepath.Dir(f.path)] {
			dirty[f.path] = true
		}
	}
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
	slices := map[string][]string{}
	for _, w := range work {
		if ws, ok := w.(workSlice); ok {
			basename := strings.TrimSuffix(ws.file.filename, filepath.Ext(ws.file.filename))
			slices[ws.f.path] = append(slices[ws.f.path], basename)
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

	for _, f := range built {
		if failed[f.path] {
			delete(mf.Folders, f.path)
			continue
		}
		sort.Strings(slices[f.path])
		mf.Folders[f.path] = folderEntry{Digest: runDigests[f.path], Slices: slices[f.path]}
	}

	return mf, true
}

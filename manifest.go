package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Bump when writer output format changes, to invalidate every cached digest.
const manifestFormatVersion = 3

const manifestFilename = ".igor-cache.json"

// The manifest as it was at the start of this run. Read-only once dirty
// detection has filled it, so the packing and writing goroutines can share it.
var priorManifest manifest

// One sliced output directory. Dir is what landed on disk, Src the source PNG
// it came from, which differ once divideAbove splits one PNG into several.
type sliceEntry struct {
	Dir string `json:"dir"`
	Src string `json:"src"`
}

// Where one piece of a source image sits on a spritesheet. Name is empty
// except for the pieces divideAbove cut out of a single PNG.
type piecePlacement struct {
	Name string `json:"name,omitempty"`
	Bin  int    `json:"bin"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

// Keyed by source filename rather than output sprite name, because output
// names shift under renameLayers and source filenames do not.
type fileRecord struct {
	File   string           `json:"file"`
	Hash   string           `json:"hash"`
	Pieces []piecePlacement `json:"pieces,omitempty"`
}

type folderEntry struct {
	Digest string       `json:"digest"`
	Bins   int          `json:"bins,omitempty"`
	Files  []fileRecord `json:"files,omitempty"`
	Slices []sliceEntry `json:"slices,omitempty"`
}

type manifest struct {
	FormatVersion int                    `json:"formatVersion"`
	Globals       string                 `json:"globals"`
	Folders       map[string]folderEntry `json:"folders"`
}

func manifestPath() string {
	return filepath.Join(prj.Destination, manifestFilename)
}

func emptyManifest() manifest {
	return manifest{
		FormatVersion: manifestFormatVersion,
		Globals:       globalsDigest(),
		Folders:       map[string]folderEntry{},
	}
}

// A missing or unreadable manifest is not an error; it just means rebuild everything.
func loadManifest() manifest {
	data, err := os.ReadFile(manifestPath())
	if err != nil {
		return emptyManifest()
	}

	var loaded manifest
	if err := json.Unmarshal(data, &loaded); err != nil {
		return emptyManifest()
	}
	if loaded.FormatVersion != manifestFormatVersion {
		return emptyManifest()
	}
	if loaded.Folders == nil {
		loaded.Folders = map[string]folderEntry{}
	}
	return loaded
}

func (m manifest) save() error {
	if err := os.MkdirAll(prj.Destination, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	// The cache is committed now, so an unchanged run must leave it alone.
	// Kept out of the written/unchanged counters, which report art.
	if old, err := os.ReadFile(manifestPath()); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return os.WriteFile(manifestPath(), data, 0644)
}

// Covers the project settings that change every folder's output.
func globalsDigest() string {
	h := sha256.New()
	fmt.Fprintf(h, "v%d\n", manifestFormatVersion)
	fmt.Fprintf(h, "sheet:%d\n", prj.SpritesheetSize)
	fmt.Fprintf(h, "slice:%d\n", prj.SliceSize)
	fmt.Fprintf(h, "prefix:%s\n", prj.ResPrefix)
	return hex.EncodeToString(h.Sum(nil))
}

// Covers a folder's file contents and its resolved rule config.
func folderDigest(f folder) string {
	lines := make([]string, 0, len(f.files))
	for _, i := range f.files {
		lines = append(lines, i.filename+" "+i.hash)
	}
	sort.Strings(lines)

	h := sha256.New()
	fmt.Fprintf(h, "cfg:%s|%t|%t|%d|%d\n", f.config.typ, f.config.renameLayers, f.config.includeCharName,
		f.config.divideAbove, f.config.alphaThreshold)
	for _, l := range lines {
		fmt.Fprintf(h, "%s\n", l)
	}
	return hex.EncodeToString(h.Sum(nil))
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Bump when writer output format changes, to invalidate every cached digest.
const manifestFormatVersion = 1

const manifestFilename = ".igor-cache.json"

type folderEntry struct {
	Digest string   `json:"digest"`
	Slices []string `json:"slices,omitempty"`
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
	return os.WriteFile(manifestPath(), append(data, '\n'), 0644)
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
	fmt.Fprintf(h, "cfg:%s|%t|%t\n", f.config.typ, f.config.renameLayers, f.config.includeCharName)
	for _, l := range lines {
		fmt.Fprintf(h, "%s\n", l)
	}
	return hex.EncodeToString(h.Sum(nil))
}

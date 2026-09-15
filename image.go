package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
)

type imageFile struct {
	filename string
	path     string
	w, h     int
	trim     trimRect
	hash     string // sha256 of file contents, for incremental builds
}

func (imf *imageFile) targetFolderPath() string {
	return filepath.Join(prj.Destination, imf.path)
}

func (imf *imageFile) load() (image.Image, error) {
	path := filepath.Join(prj.Source, imf.path, imf.filename)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return png.Decode(f)
}

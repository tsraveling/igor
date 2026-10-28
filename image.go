package main

import (
	"image"
	"image/draw"
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

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}

	// A PNG with no transparency decodes as RGBA, and an indexed one as
	// Paletted, so normalise rather than making the rest of the pipeline
	// handle every format.
	if nrgba, ok := img.(*image.NRGBA); ok {
		return nrgba, nil
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out, nil
}

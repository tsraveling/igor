package main

import (
	"image"
	"image/png"
	"os"
)

type imageFile struct {
	filename string
	path     string
	w, h     int
}

func (imf *imageFile) load() (image.Image, error) {
	f, err := os.Open(imf.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return png.Decode(f)
}

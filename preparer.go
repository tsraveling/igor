package main

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type preparedFileMsg struct {
	num int
}

type imageResult struct {
	path     string
	filename string
	w, h     int
}

func walkFiles(path string) []imageResult {
	var results []imageResult
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("!!: %s\n", err.Error())
			return err
		}
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".png") {

			relPath, _ := filepath.Rel(path, p)
			f, err := os.Open(p)
			if err != nil {
				return nil // skip this file
			}
			defer f.Close()

			img, _, err := image.DecodeConfig(f)
			if err != nil {
				fmt.Printf("Error decoding image: %s\n", err.Error())
				return nil // skip this file
			}

			results = append(results, imageResult{
				path:     relPath,
				filename: d.Name(),
				w:        img.Width,
				h:        img.Height,
			})
		}
		return nil
	})
	// FIXME: Remove this fake sleeper
	time.Sleep(200 * time.Millisecond)
	prg.Send(preparedFileMsg{num: len(results)})
	return results
}

// STUB: Use globs to dilineate files
// STUB: Check image sizes and route
// STUB: Send progress messages

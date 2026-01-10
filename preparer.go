package main

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type prepareCompleteMsg struct {
	folders []folder
}

/** Walks through the given path and assembles an array of folders, each with a child array of imageFiles. */
func walkFilesCmd(path string) tea.Cmd {
	return func() tea.Msg {
		folders := make(map[string]*folder)

		filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".png") {
				relPath, _ := filepath.Rel(path, p)
				dir := filepath.Dir(relPath)

				f, err := os.Open(p)
				if err != nil {
					return nil
				}
				defer f.Close()

				img, _, err := image.DecodeConfig(f)
				if err != nil {
					return nil
				}

				// Get or create folder
				if _, ok := folders[dir]; !ok {
					folders[dir] = &folder{
						name:  filepath.Base(dir),
						path:  dir,
						files: []imageFile{},
					}
				}

				folders[dir].files = append(folders[dir].files, imageFile{
					filename: d.Name(),
					w:        img.Width,
					h:        img.Height,
				})
			}
			return nil
		})

		// Convert map to slice
		result := make([]folder, 0, len(folders))
		for _, f := range folders {
			result = append(result, *f)
		}

		return prepareCompleteMsg{folders: result}
	}
}

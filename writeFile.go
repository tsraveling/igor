package main

import (
	"bytes"
	"os"
	"path/filepath"
)

// Writes only when the bytes differ. This is what makes a rebuilt sibling
// harmless: it still repacks, produces identical bytes, and never reaches disk
// or git status. Reports whether anything was written.
func writeIfChanged(path string, data []byte) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		sesh.FilesUnchanged.Add(1)
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return false, err
	}

	sesh.FilesWritten.Add(1)
	return true, nil
}

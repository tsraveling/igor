package main

import (
	"github.com/bmatcuk/doublestar/v4"
)

type folderType string

const (
	FolderTypeStandard  folderType = "standard"
	FolderTypeCharacter folderType = "character"
	FolderTypeEnv       folderType = "env"
)

func determineFolderType(path string) folderType {
	for pattern, rule := range prj.Rules {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return folderType(rule.Mode)
		}
	}
	return FolderTypeStandard
}

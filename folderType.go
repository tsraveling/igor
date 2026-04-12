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

type folderConfig struct {
	typ             folderType
	renameLayers    bool
	includeCharName bool
}

func getFolderConfig(path string) folderConfig {
	for pattern, rule := range prj.Rules {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return folderConfig{
				typ:             folderType(rule.Mode),
				renameLayers:    rule.RenameLayers,
				includeCharName: rule.IncludeCharName,
			}
		}
	}
	return folderConfig{typ: FolderTypeStandard}
}

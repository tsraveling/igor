package main

import (
	"github.com/bmatcuk/doublestar/v4"
)

type folderType string

const (
	FolderTypeStandard  folderType = "standard"
	FolderTypeCharacter folderType = "character"
	FolderTypeEnv       folderType = "env"
	FolderTypeItem      folderType = "item"
)

type folderConfig struct {
	typ             folderType
	renameLayers    bool
	includeCharName bool
	divideAbove     int
	alphaThreshold  int
}

// Animation frames have to stay whole, so character folders never divide.
func (c folderConfig) dividing() bool {
	return c.divideAbove > 0 && c.typ != FolderTypeCharacter
}

func getFolderConfig(path string) folderConfig {
	for pattern, rule := range prj.Rules {
		if matched, _ := doublestar.Match(pattern, path); matched {
			return folderConfig{
				typ:             folderType(rule.Mode),
				renameLayers:    rule.RenameLayers,
				includeCharName: rule.IncludeCharName,
				divideAbove:     rule.DivideAbove,
				alphaThreshold:  rule.AlphaThreshold,
			}
		}
	}
	return folderConfig{typ: FolderTypeStandard}
}

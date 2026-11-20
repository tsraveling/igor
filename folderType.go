package main

import (
	"sort"
	"strings"

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

// Ranging a map picks a random rule when two patterns match the same folder,
// and this config is resolved independently in several places -- dirty
// detection, orphan scanning, the frames writer -- so an unstable answer means
// those disagree with each other about the same folder.
//
// Ordered most specific first, so a rule naming a subfolder beats one covering
// the tree above it. Specificity is fewest wildcards, then most path segments,
// then longest; ties break alphabetically so the order is total. Documented in
// the README under Rule precedence.
func rulePatterns() []string {
	patterns := make([]string, 0, len(prj.Rules))
	for p := range prj.Rules {
		patterns = append(patterns, p)
	}

	sort.Slice(patterns, func(i, j int) bool {
		a, b := patterns[i], patterns[j]
		if wa, wb := strings.Count(a, "*"), strings.Count(b, "*"); wa != wb {
			return wa < wb
		}
		if sa, sb := strings.Count(a, "/"), strings.Count(b, "/"); sa != sb {
			return sa > sb
		}
		if len(a) != len(b) {
			return len(a) > len(b)
		}
		return a < b
	})
	return patterns
}

func getFolderConfig(path string) folderConfig {
	for _, pattern := range rulePatterns() {
		if matched, _ := doublestar.Match(pattern, path); matched {
			rule := prj.Rules[pattern]
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

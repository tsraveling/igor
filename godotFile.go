package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// The parts of an output file igor wrote before, or Godot re-saved, that are
// worth carrying into the next write. Godot stamps a uid on import and igor
// used to throw it away; reading it back off disk means the file is the
// source of truth rather than the manifest.
type godotFile struct {
	header    string            // verbatim header line, no trailing newline
	kind      string            // "gd_resource" or "gd_scene"
	typ       string            // resource type, on a gd_resource header
	uids      map[string]string // res path -> uid
	ids       map[string]string // res path -> ext_resource id
	paths     []string          // ext_resource paths, in file order
	maxID     int               // highest leading integer among the ids
	region    rect
	hasRegion bool
}

var (
	attrRe      = regexp.MustCompile(`([a-z_]+)="([^"]*)"`)
	loadStepsRe = regexp.MustCompile(`load_steps=\d+`)
	leadingRe   = regexp.MustCompile(`^\d+`)
)

func attrOf(line, key string) string {
	for _, m := range attrRe.FindAllStringSubmatch(line, -1) {
		if m[1] == key {
			return m[2]
		}
	}
	return ""
}

// Godot ids are either "3" or "3_abc12"; both start with the allocation number.
func leadingInt(id string) int {
	n, err := strconv.Atoi(leadingRe.FindString(id))
	if err != nil {
		return 0
	}
	return n
}

// A missing or unparseable file is not an error; it just means there is no
// identity to carry and the write falls back to today's behaviour.
func readGodotFile(path string) *godotFile {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	g := &godotFile{uids: map[string]string{}, ids: map[string]string{}}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "[gd_resource "), strings.HasPrefix(line, "[gd_scene "):
			if g.header != "" {
				continue
			}
			g.header = line
			g.kind = "gd_scene"
			if strings.HasPrefix(line, "[gd_resource ") {
				g.kind = "gd_resource"
			}
			g.typ = attrOf(line, "type")

		case strings.HasPrefix(line, "[ext_resource "):
			p := attrOf(line, "path")
			if p == "" {
				continue
			}
			g.paths = append(g.paths, p)
			if u := attrOf(line, "uid"); u != "" {
				g.uids[p] = u
			}
			id := attrOf(line, "id")
			if id == "" {
				continue
			}
			g.ids[p] = id
			if n := leadingInt(id); n > g.maxID {
				g.maxID = n
			}

		case strings.HasPrefix(line, "region = Rect2("):
			var r rect
			if n, _ := fmt.Sscanf(line, "region = Rect2(%d, %d, %d, %d)", &r.x, &r.y, &r.w, &r.h); n == 4 {
				g.region, g.hasRegion = r, true
			}
		}
	}

	if g.header == "" {
		return nil
	}
	return g
}

// Reuses the previous header so its uid survives. Godot and igor disagree on
// whether an AtlasTexture carries load_steps at all, and reusing the line as
// it stands settles that without modelling Godot's rule. Where the old header
// does carry a load_steps, the number is patched to this write's count, since
// it goes stale as soon as the resource count moves. igor rewrites these files
// whole, so there is never a sub_resource of its own left for that count to
// disagree with.
func carryHeader(old *godotFile, kind, typ, want string, loadSteps int) string {
	if old == nil || old.kind != kind {
		return want
	}
	if kind == "gd_resource" && old.typ != typ {
		return want
	}
	if loadStepsRe.MatchString(old.header) {
		return loadStepsRe.ReplaceAllString(old.header, fmt.Sprintf("load_steps=%d", loadSteps))
	}
	return old.header
}

// Godot's field order is type, uid, path, id. A uid is re-attached only for
// the same path it was read against: a wrong uid is worse than none.
func extResourceLine(typ, resPath, id string, old *godotFile) string {
	if old != nil && !sesh.identityWasReset(resPath) {
		if uid := old.uids[resPath]; uid != "" {
			return fmt.Sprintf("[ext_resource type=%q uid=%q path=%q id=%q]\n", typ, uid, resPath, id)
		}
	}
	return fmt.Sprintf("[ext_resource type=%q path=%q id=%q]\n", typ, resPath, id)
}

// Keeps the id the previous file gave this path, so inserting a resource never
// renumbers the ones around it. Godot does not require ids to be contiguous.
type idAllocator struct {
	old  *godotFile
	next int
}

func newIDAllocator(old *godotFile) *idAllocator {
	a := &idAllocator{old: old, next: 1}
	if old != nil {
		a.next = old.maxID + 1
	}
	return a
}

// Plain integer ids, as scenes use.
func (a *idAllocator) sceneID(resPath string) string {
	if a.old != nil {
		if id, ok := a.old.ids[resPath]; ok && id != "" {
			return id
		}
	}
	id := strconv.Itoa(a.next)
	a.next++
	return id
}

// Suffixed ids, as SpriteFrames uses.
func (a *idAllocator) resourceID(resPath string) string {
	if a.old != nil {
		if id, ok := a.old.ids[resPath]; ok && id != "" {
			return id
		}
	}
	id := fmt.Sprintf("%d_%s", a.next, generateRandomID(5))
	a.next++
	return id
}

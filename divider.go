package main

import (
	"image"
	"sort"
)

// Pieces narrower or shorter than this get a warning: they are usually export
// noise rather than something anyone meant to draw.
const minItemSize = 8

// One horizontal span of pixels above the alpha threshold.
type pixRun struct {
	y, x0, x1 int
}

type component struct {
	box  rect
	runs []pixRun
}

// Groups the pixels inside bounds into items, where an item is a cluster of
// pixels separated from every other cluster by at least gap transparent
// pixels. Returns their bounding boxes in reading order.
func divide(img *image.NRGBA, bounds rect, gap, alphaThreshold int) []rect {
	return readingOrder(mergeNearby(components(img, bounds, alphaThreshold), gap))
}

// Run-length connected components over the alpha mask. Working in spans rather
// than pixels keeps a 9000x8000 canvas to a few thousand entries instead of
// seventy million.
func components(img *image.NRGBA, bounds rect, alphaThreshold int) []component {
	uf := newUnionFind()
	var runs []pixRun
	var labels []int

	prevRuns, prevLabels := []pixRun{}, []int{}

	for y := bounds.y; y < bounds.b(); y++ {
		row := img.Pix[y*img.Stride:]
		curRuns, curLabels := []pixRun{}, []int{}

		for x := bounds.x; x < bounds.r(); {
			if int(row[x*4+3]) <= alphaThreshold {
				x++
				continue
			}
			x0 := x
			for x < bounds.r() && int(row[x*4+3]) > alphaThreshold {
				x++
			}
			r := pixRun{y: y, x0: x0, x1: x - 1}

			label := -1
			for i, p := range prevRuns {
				if p.x0 > r.x1 || p.x1 < r.x0 {
					continue
				}
				if label < 0 {
					label = uf.find(prevLabels[i])
				} else {
					uf.union(label, prevLabels[i])
				}
			}
			if label < 0 {
				label = uf.add()
			}

			runs = append(runs, r)
			labels = append(labels, label)
			curRuns = append(curRuns, r)
			curLabels = append(curLabels, label)
		}
		prevRuns, prevLabels = curRuns, curLabels
	}

	byRoot := map[int]*component{}
	var roots []int
	for i, r := range runs {
		root := uf.find(labels[i])
		c, ok := byRoot[root]
		if !ok {
			c = &component{box: rect{x: r.x0, y: r.y, w: r.x1 - r.x0 + 1, h: 1}}
			byRoot[root] = c
			roots = append(roots, root)
		}
		c.runs = append(c.runs, r)
		growTo(&c.box, r)
	}

	out := make([]component, 0, len(roots))
	for _, root := range roots {
		out = append(out, *byRoot[root])
	}
	return out
}

// Expands box to cover one more run. Runs arrive in scan order, so each
// component's run list stays sorted by y, which nearest() relies on.
func growTo(box *rect, r pixRun) {
	right, bottom := max(box.r()-1, r.x1), max(box.b()-1, r.y)
	box.x = min(box.x, r.x0)
	box.y = min(box.y, r.y)
	box.w = right - box.x + 1
	box.h = bottom - box.y + 1
}

// Two components belong to the same item when their nearest pixels are closer
// than gap. The bounding-box test rejects distant pairs before the exact scan.
func mergeNearby(comps []component, gap int) []rect {
	uf := newUnionFind()
	for range comps {
		uf.add()
	}

	for i := range comps {
		for j := i + 1; j < len(comps); j++ {
			if uf.find(i) == uf.find(j) {
				continue
			}
			if boxGap(comps[i].box, comps[j].box) >= gap {
				continue
			}
			if nearest(comps[i], comps[j], gap) < gap {
				uf.union(i, j)
			}
		}
	}

	byRoot := map[int]rect{}
	var roots []int
	for i := range comps {
		root := uf.find(i)
		box, ok := byRoot[root]
		if !ok {
			byRoot[root] = comps[i].box
			roots = append(roots, root)
			continue
		}
		byRoot[root] = union(box, comps[i].box)
	}

	out := make([]rect, 0, len(roots))
	for _, root := range roots {
		out = append(out, byRoot[root])
	}
	return out
}

// Chebyshev distance between two boxes; 0 when they overlap.
func boxGap(a, b rect) int {
	dx := max(0, b.x-(a.r()-1), a.x-(b.r()-1))
	dy := max(0, b.y-(a.b()-1), a.y-(b.b()-1))
	return max(dx, dy)
}

func union(a, b rect) rect {
	x, y := min(a.x, b.x), min(a.y, b.y)
	return rect{x: x, y: y, w: max(a.r(), b.r()) - x, h: max(a.b(), b.b()) - y}
}

// Smallest Chebyshev distance between any pixel of a and any of b, giving up
// at limit. Both run lists are sorted by y and the best distance only shrinks,
// so the window into b advances monotonically and never rescans.
func nearest(a, b component, limit int) int {
	best := limit
	j := 0

	for _, ra := range a.runs {
		for j < len(b.runs) && b.runs[j].y < ra.y-best {
			j++
		}
		for k := j; k < len(b.runs) && b.runs[k].y <= ra.y+best; k++ {
			rb := b.runs[k]

			dy := ra.y - rb.y
			if dy < 0 {
				dy = -dy
			}
			if dy >= best {
				continue
			}

			dx := 0
			if ra.x1 < rb.x0 {
				dx = rb.x0 - ra.x1
			} else if rb.x1 < ra.x0 {
				dx = ra.x0 - rb.x1
			}

			if d := max(dx, dy); d < best {
				best = d
				if best == 0 {
					return 0
				}
			}
		}
	}
	return best
}

// Reading order: the topmost ungrouped item seeds a row, every item whose
// vertical span overlaps that seed joins it, and the row reads left to right.
// Banding against the seed rather than the growing row stops a dense cloud
// from chaining itself into a single row.
func readingOrder(boxes []rect) []rect {
	sorted := make([]rect, len(boxes))
	copy(sorted, boxes)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].y != sorted[j].y {
			return sorted[i].y < sorted[j].y
		}
		return sorted[i].x < sorted[j].x
	})

	taken := make([]bool, len(sorted))
	out := make([]rect, 0, len(sorted))

	for i, seed := range sorted {
		if taken[i] {
			continue
		}
		row := []rect{}
		for j := i; j < len(sorted); j++ {
			if taken[j] {
				continue
			}
			r := sorted[j]
			if r.y <= seed.b()-1 && r.b()-1 >= seed.y {
				taken[j] = true
				row = append(row, r)
			}
		}
		sort.Slice(row, func(a, b int) bool {
			if row[a].x != row[b].x {
				return row[a].x < row[b].x
			}
			return row[a].y < row[b].y
		})
		out = append(out, row...)
	}
	return out
}

type unionFind struct {
	parent []int
}

func newUnionFind() *unionFind { return &unionFind{} }

func (u *unionFind) add() int {
	u.parent = append(u.parent, len(u.parent))
	return len(u.parent) - 1
}

func (u *unionFind) find(a int) int {
	for u.parent[a] != a {
		u.parent[a] = u.parent[u.parent[a]]
		a = u.parent[a]
	}
	return a
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}

package main

import (
	"math"
)

type MaxRectsAlgo int

const (
	// Best Short Side Fit: minimize the shorter leftover dimension.
	// Leaves a more "usable" rectangular remainder.
	AlgoBSSF MaxRectsAlgo = iota

	// Best Area Fit: minimize wasted area in this free rect.
	// Pack tightly by using rects that are close in size.
	AlgoBAF

	// Best Long Side Fit: minimize the longer leftover dimension.
	// Avoids creating long thin remainders.
	AlgoBLSF
)

type packWorkUpdateMsg struct {
	id    int
	phase packPhase
	bins  []spriteBin
}

func pack(w workPack) {

	// First, calc the bins using maxrect.
	// TODO: Use the other algos for different types of source art
	bins := maxRects(w.files, AlgoBSSF)
	prg.Send(packWorkUpdateMsg{id: w.id, phase: printing, bins: bins})
	errs := renderBins(bins, w.files)
	for _, err := range errs {
		prg.Send(toException(err, nil))
	}
}

// maxRects implements the MaxRects bin packing algorithm.
//
// Algorithm overview:
// 1. Maintain a list of maximal free rectangles (initially the entire bin)
// 2. For each sprite, find the best free rect using the chosen heuristic
// 3. Place sprite at the rect's origin and split remaining space
// 4. After splitting, remove any free rects fully contained within others
// 5. If no free rect fits the sprite, create a new bin
//
// The "maximal rectangles" approach means free rects can overlap - each
// represents the largest possible rectangle from a given corner. This
// allows better packing than guillotine cuts at the cost of more rects to track.
func maxRects(files []imageFile, algo MaxRectsAlgo) []spriteBin {
	// Start with one bin containing a single free rect spanning the full sheet
	firstBin := spriteBin{freeRects: []rect{{w: prj.SpritesheetSize, h: prj.SpritesheetSize}}}
	bins := []spriteBin{firstBin}

	for i, img := range files {
		spriteW := img.trim.w
		spriteH := img.trim.h
		placed := false

		// Try each existing bin until we find one that fits.
		// Rule is that we are trying to pack our spritesheets as full
		// as possible, which will make the packer slower but in theory
		// will save vram in game.
		for bi := range bins {
			bestScore := math.MaxInt
			bestIdx := -1

			// For each free rect:
			for fi, fr := range bins[bi].freeRects {
				// Does it fit?
				if spriteW > fr.w || spriteH > fr.h {
					continue
				}

				// Get the score with the selected algo
				score := scoreRect(fr, spriteW, spriteH, algo)
				if score < bestScore {
					bestScore = score
					bestIdx = fi
				}
			}

			// Found a suitable free rect in this bin
			if bestIdx >= 0 {
				fr := bins[bi].freeRects[bestIdx]

				// Place sprite at the bottom-left corner of the chosen free rect
				sprite := spriteRect{
					rect: rect{
						x: fr.x,
						y: fr.y,
						w: spriteW,
						h: spriteH,
					},
					i: i,
				}
				bins[bi].rects = append(bins[bi].rects, sprite)

				// Split free rects and remove the used one
				bins[bi].freeRects = splitFreeRects(bins[bi].freeRects, sprite)

				placed = true
				break
			}
		}

		// No existing bin could fit this sprite - create a new bin
		if !placed {
			newBin := spriteBin{
				freeRects: []rect{{w: prj.SpritesheetSize, h: prj.SpritesheetSize}},
			}
			sprite := spriteRect{
				rect: rect{
					x: 0,
					y: 0,
					w: spriteW,
					h: spriteH,
				},
				i: i,
			}
			newBin.rects = append(newBin.rects, sprite)
			newBin.freeRects = splitFreeRects(newBin.freeRects, sprite)
			bins = append(bins, newBin)
		}
	}
	return bins
}

// scoreRect returns a score for placing a sprite in a free rect.
// Lower scores are better. The heuristic determines what "better" means.
func scoreRect(fr rect, spriteW, spriteH int, algo MaxRectsAlgo) int {
	leftoverW := fr.w - spriteW
	leftoverH := fr.h - spriteH

	switch algo {
	case AlgoBAF:
		return (fr.w * fr.h) - (spriteW * spriteH)

	case AlgoBLSF:
		return max(leftoverW, leftoverH)

	default:
		return min(leftoverW, leftoverH)
	}
}

// splitFreeRects handles placement of a sprite by:
// 1. Finding all free rects that intersect with the placed sprite
// 2. Splitting intersecting rects into up to 4 new rects (one per side)
// 3. Removing rects that are fully contained within other rects
func splitFreeRects(freeRects []rect, placed spriteRect) []rect {
	var newFreeRects []rect

	for _, fr := range freeRects {
		// If no intersect, we're good:
		if !fr.intersects(placed.rect) {
			newFreeRects = append(newFreeRects, fr)
			continue
		}

		// Split the free rect into up to 4 new rects around the placed sprite.
		// Each new rect is the maximal rectangle extending from the original
		// free rect's edge to the placed sprite's edge.

		// Left rect: from free rect's left edge to sprite's left edge
		if placed.x > fr.x {
			newFreeRects = append(newFreeRects, rect{
				x: fr.x,
				y: fr.y,
				w: placed.x - fr.x,
				h: fr.h,
			})
		}

		// Right rect: from sprite's right edge to free rect's right edge
		if placed.x+placed.w < fr.x+fr.w {
			newFreeRects = append(newFreeRects, rect{
				x: placed.x + placed.w,
				y: fr.y,
				w: (fr.x + fr.w) - (placed.x + placed.w),
				h: fr.h,
			})
		}

		// Bottom rect: from free rect's bottom edge to sprite's bottom edge
		if placed.y > fr.y {
			newFreeRects = append(newFreeRects, rect{
				x: fr.x,
				y: fr.y,
				w: fr.w,
				h: placed.y - fr.y,
			})
		}

		// Top rect: from sprite's top edge to free rect's top edge
		if placed.y+placed.h < fr.y+fr.h {
			newFreeRects = append(newFreeRects, rect{
				x: fr.x,
				y: placed.y + placed.h,
				w: fr.w,
				h: (fr.y + fr.h) - (placed.y + placed.h),
			})
		}
	}

	// Finally, remove any rect fully contained by any other rect:
	var ret []rect
	for i, a := range newFreeRects {
		contained := false
		for j, b := range newFreeRects {
			if i != j && b.contains(a) {
				contained = true
				break
			}
		}
		if !contained {
			ret = append(ret, a)
		}
	}
	return ret
}

package main

type rect struct {
	x, y int
	w, h int
}

func (r *rect) isEmpty() bool {
	return r.w == 0 && r.h == 0
}

func (r *rect) r() int {
	return r.x + r.w
}

func (r *rect) b() int {
	return r.y + r.h
}

// returns true if a free rect and placed sprite overlap
func (fr *rect) intersects(sr rect) bool {
	return fr.x < sr.x+sr.w &&
		fr.x+fr.w > sr.x &&
		fr.y < sr.y+sr.h &&
		fr.y+fr.h > sr.y
}

// returns true if this rect fully contains rect 'inner'
func (outer *rect) contains(inner rect) bool {
	return outer.x <= inner.x &&
		outer.y <= inner.y &&
		outer.x+outer.w >= inner.x+inner.w &&
		outer.y+outer.h >= inner.y+inner.h
}

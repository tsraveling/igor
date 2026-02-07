package main

import (
	"fmt"
)

type folder struct {
	name  string
	path  string
	files []imageFile
	typ   folderType
}

type workPiece interface {
	ID() int
}

type packPhase int

const (
	calculating packPhase = iota
	printing
)

type workPack struct {
	id    int
	f     folder
	files []imageFile
	bins  []spriteBin
	phase packPhase
}

func (w workPack) ID() int {
	return w.id
}

type imgSlice struct {
	rect
	path string
}

type workSlice struct {
	id     int
	f      folder
	file   imageFile
	slices []imgSlice // Will be encoded relative to the trimmed rect
}

func (w workSlice) ID() int {
	return w.id
}

type spriteRect struct {
	rect
	i int
}

type spriteBin struct {
	rects     []spriteRect
	freeRects []rect
}

type trimRect struct {
	rect
	mR, mB int
}

func (r *trimRect) toStr() string {
	return fmt.Sprintf("%d, %d - sz: %d, %d - mrg: %d, %d", r.x, r.y, r.w, r.h, r.mR, r.mB)
}

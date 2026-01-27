package main

import "fmt"

type folder struct {
	name  string
	path  string
	files []imageFile
	typ   folderType
}

type workPiece interface {
	ID() int
}

type workPack struct {
	id    int
	f     folder
	files []imageFile
}

func (w workPack) ID() int {
	return w.id
}

type workSlice struct {
	id   int
	f    folder
	file imageFile
}

func (w workSlice) ID() int {
	return w.id
}

type spriteRect struct {
	x, y int
	w, h int
}

type spriteBin struct {
	rects []spriteRect
}

type rect struct {
	x, y   int
	w, h   int
	mR, mB int
}

func (r *rect) isEmpty() bool {
	return r.w == 0 && r.h == 0
}

func (r *rect) toStr() string {
	return fmt.Sprintf("%d, %d - sz: %d, %d - mrg: %d, %d", r.x, r.y, r.w, r.h, r.mR, r.mB)
}

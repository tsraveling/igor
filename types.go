package main

type folder struct {
	name  string
	path  string
	files []imageFile
	typ   folderType
}

type imageFile struct {
	filename string
	w, h     int
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

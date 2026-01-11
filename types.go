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

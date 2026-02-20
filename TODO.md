# Igor TODO

- [ ] ISSUES:
    - [ ] The trim etc rect is off right now -- animation jumps around
    - [ ] Some tres are not being generated at all (e.g. cast)
    - [ ] Some tres are getting cut off (e.g. jump-v2)
    - [ ] tres injection prefix in config file

- [x] Trimmer:
    - [x] Add trim rect to data: xy, wh. (combined with original size, can get offsets etc from this)
    - [x] Add get trim rect algo

- [x] Parser:
    - [x] For characters, throw exceptions if above the size limit
    - [x] For buildings, split
    - [x] Split infrastructure parsing on size limit

- [x] Packer:
    - [x] Implement MaxRects algo
    - [x] Figure out how to write to PNG, get a package if needed
    - [x] Store new quad data in model struct for encoding later
    - [x] Generate spritesheet PNGs successfully by folder
    - [x] Ensure spritesheet overflow is handled

- [ ] Improvements:
    - [x] Use a smaller sheet size if we don't need the whole thing (maybe with a margin option addable to config)
    - [x] Error handling

- [x] Slicer:
    - [x] Write slicing algo
    - [x] Render image slices to files
    - [x] Encode sliced coord data for reassembly later
    - [x] Ensure writing out to trimmed PNG chunks successfully

- [ ] Writer:
    - [ ] Packed:
        - [ ] Look at what TexturePacker2D is doing
        - [ ] Look at the file format 
        - [ ] Look into any third party .tscn / .tres writers
        - [ ] Look for the best import settings and figure out how to write that programatically
        - [ ] If not set up a generic interface
        - [ ] Generate tscns from trim data
        - [ ] Plan / document how to handle conflicts
    - [ ] Sliced:
        - [ ] Create .tres for the slices (if needed)
        - [ ] Look at Node2D with children .tscn file for reference
        - [ ] Generate assembled Node2D scene with grid layout
    - [ ] Buildings ("env"):
        - [ ] For env type folders, assemble the overall "meta" Node2D consisting of both packed sprites and sliced large images, in order to instantiate an entire building via drag and drop.
        - [ ] Figure out how to make this only "partially" overwriteable, ie if you want to add scripts or other nodes into the building Node2D. Document the solution.

- [ ] Test and see if it works
- [ ] Fix and polish until it does

## Soon

- [ ] Sprite sequence assembler


# Old Output

```go
	switch m.phase {
	case preparation:
		return fmt.Sprintf("%d folders", len(m.folders))
	case parsing:
		var b strings.Builder
		for _, f := range m.folders {
			var t string
			switch f.typ {
			case FolderTypeCharacter:
				t = "char"
			case FolderTypeEnv:
				t = "env"
			case FolderTypeStandard:
				t = "standard"
			}
			b.WriteString(f.path + " > " + t + ": " + f.name + "\n")
		}
		output := b.String()
		return fmt.Sprintf("PARSING\n\n%d files in %s:\n\n%s", len(m.folders), prj.Source, output)
	case trimming:
		var b strings.Builder
		for _, l := range m.logs {
			b.WriteString(l + "\n")
		}
		for _, exc := range m.exceptions {
			b.WriteString("ERR: " + exc.msg + "\n")
		}
		for _, i := range m.activeTrimming {
			b.WriteString(" & " + i + "\n")
		}
		output := b.String()
		return fmt.Sprintf("TRIMMING\n\n%d remaining --- %d done\n\n%s", m.numTrimPending, m.numTrimDone, output)
	case processing:
		var summaryLine = fmt.Sprintf("%d pending | %d active | %d finished", len(m.pendingWork), len(m.activeWork), len(m.finishedWork))
		var b strings.Builder
		for _, w := range m.activeWork {
			switch v := w.(type) {
			case workPack:
				switch v.phase {
				case calculating:
					b.WriteString(v.f.name + " > packing (calculating)\n")
				case printing:
					packString := fmt.Sprintf(" > packing (printing %d bins)\n", len(v.bins))
					b.WriteString(v.f.name + packString)
				}
			case workSlice:
				sliceString := fmt.Sprintf(" > slicing %d pieces\n", len(v.slices))
				b.WriteString(v.file.filename + sliceString)
			}
		}
		return fmt.Sprintf("PROCESSING\n\n%s\n\n%s", summaryLine, b.String())
	case writing:
		return "WRITING .tres resources..."
	case done:
		var b strings.Builder
		for _, w := range m.finishedWork {
			switch v := w.(type) {
			case workPack:
				packString := fmt.Sprintf(" > packed %d bins!\n", len(v.bins))
				b.WriteString(v.f.name + packString)
			case workSlice:
				sliceString := fmt.Sprintf("%s > cut %d slices!\n", v.file.filename, len(v.slices))
				b.WriteString(sliceString)
			}
		}
		return fmt.Sprintf("FINISHED!\n\n%s", b.String())
	}
	return "unsupported phase"
}

```

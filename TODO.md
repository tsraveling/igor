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

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

## Entity/Level Zippering (Art + Game Code)

The core problem: Igor owns visual/structural data (art layers, positions), while game code owns behavioral data (scripts, physics, collision, inventory logic). Both target the same .tscn files. Regenerating art can't stomp game code; updating game code can't require re-importing art.

### Recommended Approach: Godot Scene Inheritance

Igor outputs to a `generated/` directory that it fully owns and overwrites freely. Game code lives in a separate directory and uses Godot's scene inheritance to extend the generated base scenes.

```
generated/entities/glass_bottle.tscn   <- Igor owns, always overwritten
entities/glass_bottle.tscn             <- Inherits from above, game-code owned
entities/glass_bottle.gd               <- Script, physics, pickup logic, etc.

generated/levels/bar.tscn              <- Igor owns (layout, entity placement)
levels/bar.tscn                        <- Inherits, adds triggers/scripts/etc.
```

The inherited scene is a diff on top of the base. It can add nodes (Area2D, CollisionShape2D), attach scripts, and override properties (z_index, modulate) on igor-managed nodes. When Igor regenerates the base, Godot merges automatically.

Level .tscn generation references the game-layer paths (e.g. `res://entities/glass_bottle.tscn`), not the raw generated paths, so placed entities carry their game code.

**What Igor needs to do:**
- Output all generated .tscn/.tres to a `generated/` subdirectory
- Use stable node names derived from entity/layer IDs (not filenames) to prevent orphaned overrides
- Optionally scaffold stub inherited scenes on first run (protected by `--new-only`)

**Risks:**
- Node renames in the base orphan overrides in the inherited scene. Mitigated by stable IDs in the JSON abstraction.
- Node removal breaks children parented under the removed node. Same mitigation.
- Child ordering: if z-sort relies on tree order rather than `z_index` property, adding/removing layers shifts indices. Use `z_index` instead.
- Devs must know not to edit generated files directly.

**Benefits:**
- Zero merge logic in Igor -- Godot's scene system handles it natively.
- Igor stays simple: always full-overwrite its output.
- Clean ownership boundary: `generated/` = art, everything else = game code.

### Alternative Approaches

- **Parse-and-merge with markers:** Igor tags its nodes with `metadata/igor_managed = true`. On regeneration, it parses the existing .tscn, strips igor-managed nodes, splices in fresh ones, and preserves user-added nodes. Most flexible, but requires a .tscn parser and careful handling of ext_resource ID conflicts and parent-path breakage.

- **Composition via subscene:** Igor generates a layers-only subscene (e.g. `tea_shop_layers.tscn`) that gets instanced inside a user-owned wrapper scene. Clean separation, no parser needed, but adds a nesting level and makes per-layer property overrides less convenient (requires Godot's instance property override syntax).


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

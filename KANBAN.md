## BACKLOG

- Option to slice all trimmed images above the slice rect size (vs above the spritesheet size as it is now)

## IMPROVEMENTS

- Only process files that have been changed since a reference / cursor date
    * [ ] Add argument --mark-time that updates this manually
    * [ ] Probably store in the igor.yml file
    * [ ] Update every time it writes
    * [ ] When loading images, compare date to this mark and don't process unless past that.

## FIXES

- Igor strips Godot UIDs from generated resources on every rebuild, breaking `uid://` references and flooding git with diffs
    * [ ] See `_spec/preserve-uid.md`


## NEXT


## CURRENT


## DOING


## DONE

- Parser
    * [ ] Throw size error if folder is character type
    * [x] Otherwise split the streams
- Packer
    * [ ] Implement MaxRects
    * [ ] 
- Slicer
- Writer

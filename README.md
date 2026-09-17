# igor

**NOTE:** This project is still in alpha stage, and is somewhat likely to change significantly between releases.

A helpful assistant for all of the gamedev mad scientists out there. Packages sprites, generates animations, and does general pipeline work. Godot only (for now).

## Setting up a project

1. Create a file `igor.yaml` in the root of your directory (or wherever you would like to run the igor CLI tool)
2. Format:

## Config file

```
destination: ./out
source: ./in
spritesheet-size: 4096
slice-size: 1024
rules:
```

### Modes

#### Simple

`mode: simple` is set by default, so you'd only use it for subdirectories of a folder or project default set to something else.

Simple _test_ mode will:

1. Use project settings to construct spritesheets for any images under the dimension limit
2. Grid-slice images above the dimension limit, and construct a .tscn with the whole image. Note that kebab-case will be turned into snake_case to match Godot conventions.

#### Character

`mode: character` assumes that each child directory holds a different character. Each child of those holds a specific animation. A Godot SpriteFrames file will be generated for each character with all of the animations pre-set up. So you would use it like this:

```
"characters/**": { mode: character }
```

With a filestructure like this:

```
/characters
  /alice
    /jump
      ...sprite_01.png, _02.png ...
    /run
  /bob
    /idle
    /talk
```

This will generate in your output folder:

```
/characters
  /alice
    /jump
      jump.png # the spritesheet
      sprite_01.tres # the abstract texture refs
    /run
      run.png
    alice.tres # the SpriteFrames resource, which can be loaded into an AnimatedSprite2D 
```

NOTE: In character mode, large-sprite slicing does not work. If a source image exists here that is larger than the max dimension, an error will be thrown.

Character mode can also implement the **renameLayers** rule:

```
"characters/**": { mode: character, renameLayers: true }
```

If this is set, the folder name will be used as the final .tres base, rather than the source sprite PNGs. So if you have an animation layed out in frames like `A.png, B.png, C.png, D.png`, in a folder `characters/mario/jump`, they'll be named `jump_01.tres, jump_02.tres, jump_03.tres`, and so on.

```
"characters/**": { mode: character, renameLayers: true, includeCharName: true }
```

If you set `includeCharName`, then `renameLayers` will will also include the character name. For the example above, that would look like `mario_jump_01.tres`, and so on.

#### Environment

`mode: env` assumes that each subfolder contains a whole piece of environment. It will:

1. Spritesheet-pack any layers under the dimension limit
2. Automatically handle offsets
3. Grid-slice any layers above the dimension limit
4. Generate a Node2D .tscn with the whole environmental piece pre-assembled at the root.

So with:

```
/places
  /house
    roof.png, walls.png, sink.png, couch.png
```

You'll get

```
/places
  house.tscn # the whole house assembled in a Node2D you can just copy into place or use however you want
  /house
    house.png [_01, _02, etc] # spritesheets and slices
    roof_01, _02.tres, walls.tres, sink.tres # generated textures, including slices of large assetes
     
```

#### Coming Soon ...

- Grid tilesets: draw your frames in a specific configuration and Igor will automatically build a tileable spritesheet for you with all the resources
- Hex tilesets: same but with hexes
- UI 9-Slices: Define the specifics of the slice and your UI assets will be automatically be generated

```
TODO: Fill out from process established
```

## CLI mode

Igor runs its TUI when stdout is a terminal, and switches to plain text output when it is not, so piping into a file, a build script, or a CI job just works with no flags. `--cli` and `--tui` override that detection.

Process output goes to stdout, one line per phase and per finished work piece. Errors and warnings go to stderr. `--quiet` cuts stdout down to the final summary; `--verbose` adds a line per trimmed image and per cut slice.

```bash
igor --cli > build.log
igor --cli --quiet
igor --verbose | grep TOO
```

`--nuke` and `--clean` delete files and normally confirm at a prompt. CLI mode cannot prompt, so they require `--force` there, and igor refuses before touching anything on disk rather than bailing halfway through a run.

Exit codes: `0` on a clean run, `1` if any errors occurred, `130` on interrupt.

## Incremental builds

Igor tracks a content hash of every source folder in `<destination>/.igor-cache.json` and rebuilds only what changed. Commit that file so the team shares build state.

## Development

### Prerequisites

- Go 1.25+
- [cocogitto](https://github.com/cocogitto/cocogitto) for commit linting (`brew install cocogitto`)

### Commit conventions

This project uses [Conventional Commits](https://www.conventionalcommits.org/). The git hooks will validate your commit messages.

Run this to use the comitted githooks directory:

```bash
git config core.hooksPath .githooks
```

Use conventional commit syntax when making commits or PRs:

```
feat: add new feature
fix: fix a bug
docs: update documentation
chore: maintenance tasks
```

## License

[GNU GPL v3](LICENSE)



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

### Rule precedence

More than one rule can match the same folder. The most specific one wins, so a rule naming a subfolder overrides one covering the tree above it:

```
rules:
  "chars/**": { mode: character, renameLayers: true }
  "chars/hero/props": { mode: item }
```

Specificity is decided by fewest wildcards first, then most path segments, then the longer pattern, with ties broken alphabetically so the result never depends on map ordering.

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

#### Item

`mode: item` is for folders of loose props that never reassemble into anything. Unlike char and env there are no margins and no scene: each image is trimmed, packed into the folder's spritesheet, and handed back as a plain texture that draws at its own origin.

```
"items/**": { mode: item }
```

So a folder of six PNGs exported off a PSD, each sitting in a huge mostly-empty canvas, becomes one spritesheet and six `.tres` textures with the canvas offsets thrown away.

An item larger than `slice-size` is grid-sliced like any other oversized image and gets its own `.tscn`, which is what you want for a big but atomic prop such as a ship or a house.

#### divideAbove

A single PNG often holds several separate things. `divideAbove` cuts them apart:

```
"items/**": { mode: item, divideAbove: 16 }
```

Any cluster of pixels separated from every other cluster by at least that many transparent pixels becomes its own item. The value is the **minimum gap between neighbouring items**, so tune it to the tightest spacing in your art: generous for painted work, small for pixel art. Too high and neighbours fuse into one item; too low and a single object fragments.

Clusters are found by true nearest-pixel distance rather than by rows and columns, so items scattered in a cloud divide just as well as items laid out on a grid.

Pieces are named in reading order, `<source>_01`, `_02`, and so on, where a row is every piece whose vertical span overlaps the topmost piece not yet placed, read left to right. A source image that turns out to hold only one item keeps its own name with no suffix.

In `item` mode the pieces become separate textures and nothing records where they sat. In `env` and `simple` mode they are packed small and then re-added to the scene at their original positions, so a PNG of six balcony plants becomes six sprites that still line up with the rest of the building. Character mode ignores `divideAbove` and warns, since an animation frame has to stay whole.

Pieces smaller than 8x8 are kept but warned about, since they are usually export noise rather than something you drew.

#### alphaThreshold

Soft brushes and PSD exports leave a tail of nearly invisible pixels behind. A stray pixel at alpha 1 sitting between two items is enough to weld them into one, so `alphaThreshold` sets the alpha a pixel has to exceed before `divideAbove` counts it:

```
"items/**": { mode: item, divideAbove: 16, alphaThreshold: 4 }
```

It defaults to `0`, meaning any pixel above fully transparent counts. Raise it when items that are clearly apart in the source keep merging. It applies only to division, never to the ordinary trimming pass.

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
igor --verbose 2>&1 | grep TOO
```

`--nuke` and `--clean` delete files and normally confirm at a prompt. CLI mode cannot prompt, so they require `--force` there, and igor refuses before touching anything on disk rather than bailing halfway through a run. A prompt that cannot be answered, such as one with stdin redirected, is an error rather than a silent no.

Exit codes: `0` on a clean run, `1` if any errors occurred, `130` on ctrl+c, `143` on SIGTERM. A build cache that cannot be written counts as an error, since the next run would otherwise silently rebuild everything.

## Incremental builds

Igor tracks a content hash of every source folder in `<destination>/.igor-cache.json` and rebuilds only what changed.

**Commit `.igor-cache.json`.** It is no longer just a speed cache. Alongside the hashes it stores where every sprite sits on its spritesheet, and igor honours those slots on the next run so that editing one image does not reflow the sheet around it. A clone with no cache has no layout to honour. Igor will recover what it can by reading the regions out of the `.tres` files already in the output tree, but a clone with neither will repack from scratch, which is exactly the churn incremental layout exists to avoid. The cache now shows up in art diffs, but only the entries for the folder you actually touched will move.

Adding or changing an image produces diff only for that image and the things that directly reference it:

- Editing one animation frame rewrites that frame's `.tres` and its spritesheet. The other frames, the other animations, and every sibling's region stay byte-identical.
- Replacing one PNG in an environment package redraws that sprite's region on the sheet and leaves every sibling `.tres` alone.
- A run over unchanged art writes nothing at all. Igor renders each file and compares it against what is already on disk, so a rebuilt folder that produces identical bytes never reaches `git status`.

### The character frames resource is generated, not yours

`{char}_frames.tres` is written from scratch on every run that touches the character, with one animation per folder, igor's default speed, and loop on. It is a SpriteFrames sitting in your project, so it is the obvious place to tune a speed or compose an animation out of frames that already exist -- and the next run will overwrite all of it.

Keep the version you have tuned somewhere igor does not write, next to the scene that uses it, and copy across from the generated one when the frame lists change. Igor warns when the file it is about to overwrite looks hand edited: an animation whose name does not match a source folder, or a speed or loop it would not have written itself.

Igor also keeps Godot's identity across rebuilds. It reads the `uid://` and the `ext_resource` ids out of the file it is about to overwrite and re-attaches them, so references from your scenes survive a rebuild. `--clean` and `--nuke` delete the output tree and lose those uids with it; Godot mints new ones on the next import.

Incremental placement leaves holes behind over time. Igor never compacts a sheet on its own — a silent reflow is the thing all of this exists to prevent — but it will name folders where a fresh pack would do meaningfully better. `--repack` ignores the stored layout and repacks every folder; `--nuke` implies it, since it deletes the layout along with the output. `--clean` does not: the folders it leaves standing keep theirs.

One case cannot hold the line. Under `renameLayers`, output is named by alphabetical position, so inserting a frame into the *middle* of an animation renames every later one. Those are new resources with new uids, and any scene pointing at the old names will break. Igor prints the full rename list when it happens rather than letting it pass quietly.

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



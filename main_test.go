package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The repo's own test/ folder is gitignored, so every case builds its own
// fixture instead.
const testConfig = `destination: ./out
source: ./in
spritesheet-size: 64
slice-size: 32
res-prefix: out/
rules:
  "chars/**": { mode: character }
  "env/**": { mode: env }
`

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "igor-bin")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "igor")

	build := []string{"build"}
	if raceEnabled {
		build = append(build, "-race")
	}
	build = append(build, "-o", binPath, ".")

	out, err := exec.Command("go", build...).CombinedOutput()
	if err != nil {
		panic("building igor: " + err.Error() + "\n" + string(out))
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// Writes a PNG with an opaque block inset from the edges, so trimming has
// something to find and the image is never fully transparent.
func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			img.Set(x, y, color.NRGBA{R: 200, G: 40, B: 90, A: 255})
		}
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func writeImage(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// Builds a project with one character animation and one env folder. With
// oversized set, the env folder also gets an image too big to pack, which
// exercises the TOO LARGE path.
func fixture(t *testing.T, oversized bool) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"jump-01.png", "jump-02.png", "jump-03.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "jump", name), 16, 16)
	}
	writePNG(t, filepath.Join(dir, "in", "env", "house", "roof.png"), 20, 20)
	writePNG(t, filepath.Join(dir, "in", "env", "house", "walls.png"), 24, 24)

	if oversized {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "jump", "huge.png"), 128, 128)
	}

	return dir
}

type result struct {
	stdout string
	stderr string
	code   int
}

func runIgor(t *testing.T, dir string, args ...string) result {
	t.Helper()

	cmd := exec.Command(binPath, append([]string{dir}, args...)...)
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	// Stdout is a pipe here rather than a terminal, which is what makes the
	// autodetect cases meaningful.
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}

	return result{stdout: out.String(), stderr: errBuf.String(), code: code}
}

func TestCLIRunSucceeds(t *testing.T) {
	r := runIgor(t, fixture(t, false), "--cli")

	if r.code != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}

	for _, banner := range []string{"[preparation]", "[trimming]", "[parsing]", "[processing]", "[writing]"} {
		if !strings.Contains(r.stdout, banner) {
			t.Errorf("stdout missing %s banner:\n%s", banner, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "igor ") {
		t.Errorf("stdout missing version line:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "0 errors") {
		t.Errorf("stdout missing clean summary:\n%s", r.stdout)
	}
}

// A PNG with no transparent pixel decodes as RGBA and an indexed one as
// Paletted, neither of which is the NRGBA the trimmer wants.
func TestNonNRGBASourcesAreHandled(t *testing.T) {
	dir := fixture(t, false)
	base := filepath.Join(dir, "in", "chars", "bob", "idle")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}

	opaque := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			opaque.Set(x, y, color.RGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	writeImage(t, filepath.Join(base, "idle-01.png"), opaque)

	paletted := image.NewPaletted(image.Rect(0, 0, 16, 16), color.Palette{color.Transparent, color.RGBA{9, 9, 9, 255}})
	for y := 4; y < 12; y++ {
		for x := 4; x < 12; x++ {
			paletted.SetColorIndex(x, y, 1)
		}
	}
	writeImage(t, filepath.Join(base, "idle-02.png"), paletted)

	r := runIgor(t, dir, "--cli")
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "chars", "bob", "idle")); err != nil {
		t.Errorf("opaque and paletted sources should have been packed: %s", err)
	}
}

func TestAutodetectsCLIWhenPiped(t *testing.T) {
	r := runIgor(t, fixture(t, false))

	if r.code != 0 {
		t.Fatalf("exit %d, want 0\nstderr:\n%s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "[processing]") {
		t.Errorf("expected CLI output without --cli, got:\n%s", r.stdout)
	}
}

func TestQuietPrintsSummaryOnly(t *testing.T) {
	r := runIgor(t, fixture(t, false), "--cli", "--quiet")

	if r.code != 0 {
		t.Fatalf("exit %d, want 0\nstderr:\n%s", r.code, r.stderr)
	}
	if strings.Contains(r.stdout, "[processing]") {
		t.Errorf("quiet mode printed phase banners:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "0 errors") {
		t.Errorf("quiet mode dropped the summary:\n%s", r.stdout)
	}
}

func TestVerboseAddsDetail(t *testing.T) {
	r := runIgor(t, fixture(t, false), "--cli", "--verbose")

	if r.code != 0 {
		t.Fatalf("exit %d, want 0\nstderr:\n%s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, "trimmed ") {
		t.Errorf("verbose mode missing per-image trim lines:\n%s", r.stdout)
	}
}

func TestOversizedImageFailsRun(t *testing.T) {
	dir := fixture(t, true)
	r := runIgor(t, dir, "--cli")

	if r.code != 1 {
		t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "[TOO LARGE]") {
		t.Errorf("expected the exception on stderr, got:\n%s", r.stderr)
	}
	if strings.Contains(r.stdout, "[TOO LARGE]") {
		t.Errorf("exception leaked onto stdout:\n%s", r.stdout)
	}
	// Writing any of the folder would renumber the frames around the one that
	// could not be packed.
	if _, err := os.Stat(filepath.Join(dir, "out")); !os.IsNotExist(err) {
		t.Errorf("nothing should have been written after a pack failure")
	}
}

func TestUntrimmableImageFailsRun(t *testing.T) {
	dir := fixture(t, false)

	// Fully transparent, so there is no trim rect to find.
	empty := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	f, err := os.Create(filepath.Join(dir, "in", "chars", "alice", "jump", "blank.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, empty); err != nil {
		t.Fatal(err)
	}
	f.Close()

	r := runIgor(t, dir, "--cli")
	if r.code != 1 {
		t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "blank.png") {
		t.Errorf("error should name the offending image, got:\n%s", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); !os.IsNotExist(err) {
		t.Errorf("nothing should have been written after a trim failure")
	}
}

func TestTUINukeWithoutTerminalFails(t *testing.T) {
	dir := fixture(t, false)

	if r := runIgor(t, dir, "--cli"); r.code != 0 {
		t.Fatalf("setup run failed: %d\n%s", r.code, r.stderr)
	}

	// --tui keeps CLI mode's stdout check from firing, so this exercises the
	// stdin check instead. Stdin here is not a terminal.
	r := runIgor(t, dir, "--tui", "--nuke")
	if r.code == 0 {
		t.Fatalf("a prompt that cannot be answered should not exit 0\nstdout:\n%s", r.stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err != nil {
		t.Errorf("output should be intact after a refused nuke: %s", err)
	}
}

func TestSignalExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		sig  os.Signal
		want int
	}{
		{"interrupt", os.Interrupt, 130},
		{"terminate", syscall.SIGTERM, 143},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixture(t, false)

			// Enough pixels that trimming alone outlasts signal delivery on any
			// machine, so the assertion below is never skipped.
			for i := range 40 {
				writePNG(t, filepath.Join(dir, "in", "env", "town", fmt.Sprintf("layer_%03d.png", i)), 900, 900)
			}

			cmd := exec.Command(binPath, dir, "--cli")
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}

			// The banner proves the handler is installed. It does not prove work
			// remains, so the fixture above is sized so that trimming alone far
			// outlasts signal delivery.
			reader := bufio.NewReader(stdout)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					t.Fatalf("run ended before it could be signalled: %s", err)
				}
				if strings.Contains(line, "[trimming]") {
					break
				}
			}

			if err := cmd.Process.Signal(tc.sig); err != nil {
				t.Fatalf("signalling: %s", err)
			}

			ee, ok := cmd.Wait().(*exec.ExitError)
			if !ok {
				t.Fatal("run finished before the signal landed; the fixture is too small for this machine")
			}
			if ee.ExitCode() != tc.want {
				t.Errorf("exit %d, want %d", ee.ExitCode(), tc.want)
			}
		})
	}
}

func TestRejectsMultiplePaths(t *testing.T) {
	dir := fixture(t, false)

	r := runIgor(t, dir, dir, "--cli")
	if r.code == 0 {
		t.Fatal("two paths should be rejected")
	}
	if !strings.Contains(r.stderr, "at most one path") {
		t.Errorf("unhelpful error: %s", r.stderr)
	}
}

func TestNukeRequiresForceInCLI(t *testing.T) {
	dir := fixture(t, false)

	if r := runIgor(t, dir, "--cli"); r.code != 0 {
		t.Fatalf("setup run failed: %d\n%s", r.code, r.stderr)
	}
	before, err := os.ReadDir(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}

	r := runIgor(t, dir, "--cli", "--nuke")
	if r.code == 0 {
		t.Fatalf("--nuke without --force should fail in CLI mode\nstdout:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "--force") {
		t.Errorf("error should point at --force, got: %s", r.stderr)
	}

	after, err := os.ReadDir(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("output directory was removed: %s", err)
	}
	if len(after) != len(before) {
		t.Errorf("output directory changed: %d entries before, %d after", len(before), len(after))
	}
}

func TestCleanRequiresForceInCLI(t *testing.T) {
	r := runIgor(t, fixture(t, false), "--cli", "--clean")

	if r.code == 0 {
		t.Fatal("--clean without --force should fail in CLI mode")
	}
	if !strings.Contains(r.stderr, "--force") {
		t.Errorf("error should point at --force, got: %s", r.stderr)
	}
}

func TestQuietSuppressesPreRunMessages(t *testing.T) {
	dir := fixture(t, false)

	if r := runIgor(t, dir, "--cli"); r.code != 0 {
		t.Fatalf("setup run failed: %d\n%s", r.code, r.stderr)
	}

	r := runIgor(t, dir, "--cli", "--quiet", "--nuke", "--force")
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\nstderr:\n%s", r.code, r.stderr)
	}
	if strings.Contains(r.stdout, "Nuked") {
		t.Errorf("quiet mode printed the nuke message:\n%s", r.stdout)
	}
	if lines := strings.Count(strings.TrimSpace(r.stdout), "\n"); lines != 0 {
		t.Errorf("quiet mode should print one line, got:\n%s", r.stdout)
	}
}

func TestExceptionPathIsResolvable(t *testing.T) {
	dir := fixture(t, true)
	r := runIgor(t, dir, "--cli")

	if !strings.Contains(r.stderr, filepath.Join("in", "chars", "alice", "jump", "huge.png")) {
		t.Errorf("exception path should be resolvable from the project dir, got:\n%s", r.stderr)
	}
}

// Work pieces used to be dropped when the queue slice shifted underneath the
// goroutine launching them, which silently lost whole folders of output.
func TestOutputIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(testConfig), 0644); err != nil {
		t.Fatal(err)
	}
	for f := range 14 {
		for i := range 5 {
			writePNG(t, filepath.Join(dir, "in", "chars", "alice", fmt.Sprintf("anim%02d", f), fmt.Sprintf("f-%02d.png", i)), 16, 16)
		}
	}

	var first []string
	for run := range 4 {
		if err := os.RemoveAll(filepath.Join(dir, "out")); err != nil {
			t.Fatal(err)
		}
		if r := runIgor(t, dir, "--cli"); r.code != 0 {
			t.Fatalf("run %d: exit %d\n%s", run, r.code, r.stderr)
		}

		var got []string
		err := filepath.WalkDir(filepath.Join(dir, "out"), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				got = append(got, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(got)

		if first == nil {
			first = got
			continue
		}
		if !slices.Equal(first, got) {
			t.Fatalf("run %d produced %d files, first run produced %d", run, len(got), len(first))
		}
	}
}

func TestConflictingFlags(t *testing.T) {
	dir := fixture(t, false)

	for _, args := range [][]string{{"--cli", "--tui"}, {"--quiet", "--verbose"}} {
		r := runIgor(t, dir, args...)
		if r.code == 0 {
			t.Errorf("%v should be rejected", args)
		}
	}
}

// One solid rectangle of a fixture image. Alpha 0 means fully opaque, so the
// common case stays terse.
type block struct {
	x, y, w, h int
	alpha      uint8
}

// Draws blocks on a transparent canvas, giving divideAbove known clusters.
func writeBlocks(t *testing.T, path string, w, h int, blocks []block) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for _, b := range blocks {
		a := b.alpha
		if a == 0 {
			a = 255
		}
		for y := b.y; y < b.y+b.h; y++ {
			for x := b.x; x < b.x+b.w; x++ {
				img.Set(x, y, color.NRGBA{R: 200, G: 40, B: 90, A: a})
			}
		}
	}
	writeImage(t, path, img)
}

func projectWith(t *testing.T, rules string) string {
	t.Helper()
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 128\nslice-size: 64\nres-prefix: out/\nrules:\n" + rules
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The atlas region a .tres points at. Pieces are given distinct sizes in these
// fixtures, so the region identifies which one landed where.
func regionOf(t *testing.T, path string) rect {
	t.Helper()
	var r rect
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if n, _ := fmt.Sscanf(line, "region = Rect2(%d, %d, %d, %d)", &r.x, &r.y, &r.w, &r.h); n == 4 {
			return r
		}
	}
	t.Fatalf("no region in %s", path)
	return r
}

func mustRun(t *testing.T, dir string, args ...string) result {
	t.Helper()
	r := runIgor(t, dir, args...)
	if r.code != 0 {
		t.Fatalf("exit %d, want 0\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	return r
}

// Four blocks arranged so that strict top-to-bottom order and reading order
// disagree: the tall block starts highest, but the small one to its left
// shares its row and so comes first.
func readingOrderFixture(t *testing.T, rules string) string {
	t.Helper()
	dir := projectWith(t, rules)
	writeBlocks(t, filepath.Join(dir, "in", "items", "props", "cluster.png"), 100, 100, []block{
		{x: 50, y: 0, w: 10, h: 30},
		{x: 5, y: 10, w: 9, h: 9},
		{x: 70, y: 60, w: 12, h: 12},
		{x: 5, y: 65, w: 11, h: 11},
	})
	return dir
}

func TestItemModeDividesInReadingOrder(t *testing.T) {
	dir := readingOrderFixture(t, "  \"items/**\": { mode: item, divideAbove: 10 }\n")
	mustRun(t, dir, "--cli")

	// Row one is the 9x9 then the 10x30; row two the 11x11 then the 12x12.
	want := []rect{{w: 9, h: 9}, {w: 10, h: 30}, {w: 11, h: 11}, {w: 12, h: 12}}
	for i, w := range want {
		name := fmt.Sprintf("cluster_%02d.tres", i+1)
		got := regionOf(t, filepath.Join(dir, "out", "items", "props", name))
		if got.w != w.w || got.h != w.h {
			t.Errorf("%s is %dx%d, want %dx%d", name, got.w, got.h, w.w, w.h)
		}
	}
}

func TestItemModeOmitsMargin(t *testing.T) {
	dir := readingOrderFixture(t, "  \"items/**\": { mode: item, divideAbove: 10 }\n")
	mustRun(t, dir, "--cli")

	for i := 1; i <= 4; i++ {
		name := fmt.Sprintf("cluster_%02d.tres", i)
		if body := readFile(t, filepath.Join(dir, "out", "items", "props", name)); strings.Contains(body, "margin") {
			t.Errorf("%s carries a margin:\n%s", name, body)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "items", "props", "props.tscn")); err == nil {
		t.Error("item mode wrote a scene")
	}
}

func TestItemModeWithoutDivideKeepsWholeImage(t *testing.T) {
	dir := projectWith(t, "  \"items/**\": { mode: item }\n")
	writeBlocks(t, filepath.Join(dir, "in", "items", "props", "plant.png"), 100, 100, []block{
		{x: 5, y: 5, w: 10, h: 10},
		{x: 45, y: 45, w: 12, h: 12},
	})
	mustRun(t, dir, "--cli")

	// No divideAbove, so the two clusters stay one texture spanning both.
	got := regionOf(t, filepath.Join(dir, "out", "items", "props", "plant.tres"))
	if got.w != 52 || got.h != 52 {
		t.Errorf("region is %dx%d, want 52x52", got.w, got.h)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "items", "props", "plant_01.tres")); err == nil {
		t.Error("undivided image was split anyway")
	}
}

// A chain of near-invisible pixels between two blocks is enough to weld them
// into one item, which is what alphaThreshold exists to ignore.
func bridgedFixture(t *testing.T, rules string) string {
	t.Helper()
	dir := projectWith(t, rules)
	writeBlocks(t, filepath.Join(dir, "in", "items", "props", "bridged.png"), 64, 40, []block{
		{x: 10, y: 10, w: 10, h: 10},
		{x: 40, y: 10, w: 10, h: 10},
		{x: 25, y: 14, w: 1, h: 1, alpha: 2},
		{x: 33, y: 14, w: 1, h: 1, alpha: 2},
	})
	return dir
}

func TestFaintPixelsBridgeItemsByDefault(t *testing.T) {
	dir := bridgedFixture(t, "  \"items/**\": { mode: item, divideAbove: 10 }\n")
	mustRun(t, dir, "--cli")

	// One piece, so it keeps the source name rather than taking a suffix.
	if _, err := os.Stat(filepath.Join(dir, "out", "items", "props", "bridged.tres")); err != nil {
		t.Errorf("expected a single welded item: %v", err)
	}
}

func TestAlphaThresholdSeparatesBridgedItems(t *testing.T) {
	dir := bridgedFixture(t, "  \"items/**\": { mode: item, divideAbove: 10, alphaThreshold: 4 }\n")
	mustRun(t, dir, "--cli")

	for _, name := range []string{"bridged_01.tres", "bridged_02.tres"} {
		if _, err := os.Stat(filepath.Join(dir, "out", "items", "props", name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	for i, want := range []int{10, 10} {
		name := fmt.Sprintf("bridged_%02d.tres", i+1)
		if got := regionOf(t, filepath.Join(dir, "out", "items", "props", name)); got.w != want {
			t.Errorf("%s is %d wide, want %d", name, got.w, want)
		}
	}
}

func TestDivideAboveIgnoredInCharacterMode(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, divideAbove: 10 }\n")
	for _, name := range []string{"jump-01.png", "jump-02.png"} {
		writeBlocks(t, filepath.Join(dir, "in", "chars", "alice", "jump", name), 64, 40, []block{
			{x: 5, y: 5, w: 10, h: 10},
			{x: 40, y: 5, w: 10, h: 10},
		})
	}
	r := mustRun(t, dir, "--cli")

	if !strings.Contains(r.stderr, "divideAbove ignored in character mode") {
		t.Errorf("no warning on stderr:\n%s", r.stderr)
	}
	// Each frame stays whole, spanning both blocks.
	got := regionOf(t, filepath.Join(dir, "out", "chars", "alice", "jump", "jump-01.tres"))
	if got.w != 45 {
		t.Errorf("frame is %d wide, want 45 (undivided)", got.w)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "chars", "alice", "jump", "jump-01_01.tres")); err == nil {
		t.Error("an animation frame was divided")
	}
}

// In env mode the pieces keep canvas-relative margins, so the scene can place
// every one of them at the origin and still have the art line up.
func TestDividedPiecesRejoinEnvScene(t *testing.T) {
	dir := projectWith(t, "  \"env/**\": { mode: env, divideAbove: 10 }\n")
	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "decor.png"), 100, 60, []block{
		{x: 5, y: 5, w: 10, h: 10},
		{x: 60, y: 30, w: 12, h: 12},
	})
	mustRun(t, dir, "--cli")

	first := readFile(t, filepath.Join(dir, "out", "env", "house", "decor_01.tres"))
	if !strings.Contains(first, "margin = Rect2(5, 5, 90, 50)") {
		t.Errorf("decor_01 lost its canvas offset:\n%s", first)
	}
	second := readFile(t, filepath.Join(dir, "out", "env", "house", "decor_02.tres"))
	if !strings.Contains(second, "margin = Rect2(60, 30, 88, 48)") {
		t.Errorf("decor_02 lost its canvas offset:\n%s", second)
	}

	scene := readFile(t, filepath.Join(dir, "out", "env", "house", "house.tscn"))
	for _, node := range []string{"name=\"decor_01\"", "name=\"decor_02\""} {
		if !strings.Contains(scene, node) {
			t.Errorf("scene missing %s:\n%s", node, scene)
		}
	}
}

// A divided image's slice directories are named for the piece, not the source
// PNG, so stale detection has to remember which source each one came from.
func TestStaleDetectionFollowsDividedSlices(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 128\nslice-size: 16\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env, divideAbove: 10 }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	decor := filepath.Join(dir, "in", "env", "house", "decor.png")
	writeBlocks(t, decor, 100, 60, []block{
		{x: 5, y: 5, w: 20, h: 20},
		{x: 60, y: 30, w: 20, h: 20},
	})
	writePNG(t, filepath.Join(dir, "in", "env", "house", "roof.png"), 10, 10)
	mustRun(t, dir, "--cli")

	// Both pieces exceed slice-size, so each gets its own directory.
	for _, name := range []string{"decor_01", "decor_02"} {
		if _, err := os.Stat(filepath.Join(dir, "out", "env", "house", name)); err != nil {
			t.Fatalf("missing slice dir %s: %v", name, err)
		}
	}

	// A run that still has the source must leave those directories alone.
	mustRun(t, dir, "--cli", "--clean", "--force")
	for _, name := range []string{"decor_01", "decor_02"} {
		if _, err := os.Stat(filepath.Join(dir, "out", "env", "house", name)); err != nil {
			t.Errorf("%s removed while its source was still present: %v", name, err)
		}
	}

	if err := os.Remove(decor); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli", "--clean", "--force")
	for _, name := range []string{"decor_01", "decor_02"} {
		if _, err := os.Stat(filepath.Join(dir, "out", "env", "house", name)); err == nil {
			t.Errorf("%s survived the removal of its source", name)
		}
	}
}

// --- Incremental layout and identity ------------------------------------

// Modification times across the output tree, for proving which files a run
// actually rewrote. writeIfChanged means an untouched file keeps its mtime.
func snapshot(t *testing.T, root string) map[string]time.Time {
	t.Helper()
	out := map[string]time.Time{}

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[rel] = info.ModTime()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Paths that were written or created between the two snapshots.
func rewritten(before, after map[string]time.Time) []string {
	var out []string
	for p, now := range after {
		if was, ok := before[p]; !ok || !was.Equal(now) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Every region rect in the output tree, keyed by the .tres that carries it.
func regionsIn(t *testing.T, root string) map[string]rect {
	t.Helper()
	out := map[string]rect{}

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".tres" {
			return nil
		}
		for _, line := range strings.Split(readFile(t, p), "\n") {
			var r rect
			if n, _ := fmt.Sscanf(line, "region = Rect2(%d, %d, %d, %d)", &r.x, &r.y, &r.w, &r.h); n == 4 {
				rel, _ := filepath.Rel(root, p)
				out[rel] = r
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Rewrites the output tree the way Godot does on import: a uid on every header
// and every ext_resource, igor's load_steps dropped from AtlasTextures, and
// one added to the SpriteFrames resource that igor leaves without. A reference
// carries the same uid as the header of the file it points at, as Godot's do.
func stampUIDs(t *testing.T, root string) {
	t.Helper()

	n := 0
	nextUID := func() string {
		n++
		return fmt.Sprintf("uid://igortest%05d", n)
	}

	resPathOf := func(p string) string {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatal(err)
		}
		return "res://out/" + filepath.ToSlash(rel)
	}

	// Every file Godot would assign an identity to, resources and imported
	// images alike, so a reference can be given the uid its target carries.
	uids := map[string]string{}
	resources := []string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch ext := filepath.Ext(p); {
		case d.IsDir():
		case ext == ".tres" || ext == ".tscn":
			uids[resPathOf(p)] = nextUID()
			resources = append(resources, p)
		case ext == ".png":
			uids[resPathOf(p)] = nextUID()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(resources)

	for _, p := range resources {
		lines := strings.Split(readFile(t, p), "\n")
		exts := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "[ext_resource ") {
				exts++
			}
		}

		for i, line := range lines {
			switch {
			case strings.HasPrefix(line, "[gd_resource "), strings.HasPrefix(line, "[gd_scene "):
				if strings.Contains(line, `type="AtlasTexture"`) {
					line = strings.Replace(line, " load_steps=2", "", 1)
				}
				if strings.Contains(line, `type="SpriteFrames"`) {
					line = strings.Replace(line, " format=3",
						fmt.Sprintf(" load_steps=%d format=3", exts+1), 1)
				}
				lines[i] = strings.TrimSuffix(line, "]") + fmt.Sprintf(" uid=%q]", uids[resPathOf(p)])
			case strings.HasPrefix(line, "[ext_resource "):
				target := uids[attrOf(line, "path")]
				if target == "" {
					target = nextUID()
				}
				lines[i] = strings.Replace(line, " path=", fmt.Sprintf(" uid=%q path=", target), 1)
			}
		}

		if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// One character with two animations, plus an environment package whose layers
// all trim to different sizes so a region identifies which sprite is where.
func layoutFixture(t *testing.T) string {
	t.Helper()
	dir := projectWith(t, "  \"env/**\": { mode: env }\n  \"chars/**\": { mode: character }\n")

	for i, sz := range []int{10, 12, 14, 16} {
		writeBlocks(t, filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i)),
			40, 40, []block{{x: 5, y: 5, w: sz, h: sz}})
	}
	for _, name := range []string{"jump-01.png", "jump-02.png", "jump-03.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "jump", name), 16, 16)
	}
	for _, name := range []string{"run-01.png", "run-02.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 18, 18)
	}
	return dir
}

// The whole point: a rebuild of unchanged art reproduces what Godot left on
// disk, byte for byte, uids included.
func TestRebuildIsIdempotentAfterGodotImport(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")

	stampUIDs(t, out)
	before := snapshot(t, out)

	r := mustRun(t, dir, "--cli", "--all")
	if changed := rewritten(before, snapshot(t, out)); len(changed) > 0 {
		t.Errorf("a rebuild of unchanged art rewrote %d file(s): %v", len(changed), changed)
	}
	if !strings.Contains(r.stdout, "0 files written") {
		t.Errorf("summary should report nothing written:\n%s", r.stdout)
	}
}

// Godot's uid and igor's ext_resource ids both have to survive a rebuild, or
// every scene referencing the resource goes stale.
func TestUIDsAndIDsSurviveRebuild(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	frames := filepath.Join(out, "chars", "alice", "alice_frames.tres")
	wantFrames := readFile(t, frames)
	tres := filepath.Join(out, "chars", "alice", "jump", "jump-01.tres")
	wantTres := readFile(t, tres)

	mustRun(t, dir, "--cli", "--all")

	if got := readFile(t, tres); got != wantTres {
		t.Errorf("AtlasTexture lost its identity:\nwant:\n%s\ngot:\n%s", wantTres, got)
	}
	if got := readFile(t, frames); got != wantFrames {
		t.Errorf("SpriteFrames lost its identity:\nwant:\n%s\ngot:\n%s", wantFrames, got)
	}
	if !strings.Contains(wantTres, "uid://") {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}
}

// Editing one animation frame used to rewrite every sibling animation in the
// character, through the frames.tres that was built from the work queue.
func TestOneFrameEditStaysInItsFolder(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	before := snapshot(t, out)

	// Same trimmed size, different pixels: the sheet is redrawn, nothing moves.
	writeBlocks(t, filepath.Join(dir, "in", "chars", "alice", "jump", "jump-02.png"),
		16, 16, []block{{x: 1, y: 1, w: 14, h: 14, alpha: 128}})
	mustRun(t, dir, "--cli")

	for _, p := range rewritten(before, snapshot(t, out)) {
		if p == manifestFilename {
			continue
		}
		if !strings.HasPrefix(p, filepath.Join("chars", "alice", "jump")+string(filepath.Separator)) {
			t.Errorf("editing one frame rewrote %s", p)
		}
	}
}

// Content decides whether a region is redrawn; only size decides whether it
// moves. An artist's textural tweak to one layer used to reflow the package.
func TestEnvLayerEditKeepsEveryRegionIncludingItsOwn(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	edited := filepath.Join(out, "env", "house", "layer_01.tres")

	mustRun(t, dir, "--cli")
	before := regionsIn(t, out)
	wasBody := readFile(t, edited)

	// Same trimmed size, different alpha bbox: the margin moves, the slot does not.
	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "layer_01.png"),
		40, 40, []block{{x: 20, y: 18, w: 12, h: 12}})
	mustRun(t, dir, "--cli")

	if readFile(t, edited) == wasBody {
		t.Fatal("the edited layer's .tres did not change, so this proves nothing")
	}

	after := regionsIn(t, out)
	for p, was := range before {
		if after[p] != was {
			t.Errorf("%s moved from %v to %v", p, was, after[p])
		}
	}
	if len(after) != len(before) {
		t.Errorf("sprite count changed: %d -> %d", len(before), len(after))
	}
}

// A sprite that outgrows its slot has to move. Everything else has to not.
func TestGrownSpriteMovesAlone(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	before := regionsIn(t, out)

	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "layer_01.png"),
		40, 40, []block{{x: 2, y: 2, w: 30, h: 30}})
	mustRun(t, dir, "--cli")

	after := regionsIn(t, out)
	grown := filepath.Join("env", "house", "layer_01.tres")
	if after[grown] == before[grown] {
		t.Errorf("the grown sprite kept its old slot %v", before[grown])
	}
	for p, was := range before {
		if p == grown {
			continue
		}
		if after[p] != was {
			t.Errorf("%s moved from %v to %v after a sibling grew", p, was, after[p])
		}
	}
}

// Adding an animation must not disturb the ones already there, and must not
// renumber the ext_resource ids in the character's frames.tres.
func TestNewAnimationLeavesExistingOnesAlone(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	frames := filepath.Join(out, "chars", "alice", "alice_frames.tres")
	wasIDs := extIDsOf(t, frames)
	before := snapshot(t, out)

	for _, name := range []string{"idle-01.png", "idle-02.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "idle", name), 12, 12)
	}
	mustRun(t, dir, "--cli")

	for _, p := range rewritten(before, snapshot(t, out)) {
		switch {
		case p == manifestFilename,
			p == filepath.Join("chars", "alice", "alice_frames.tres"),
			strings.HasPrefix(p, filepath.Join("chars", "alice", "idle")+string(filepath.Separator)):
			continue
		}
		t.Errorf("adding an animation rewrote %s", p)
	}

	nowIDs := extIDsOf(t, frames)
	for path, id := range wasIDs {
		if nowIDs[path] != id {
			t.Errorf("%s was renumbered from %q to %q", path, id, nowIDs[path])
		}
	}
	if len(nowIDs) != len(wasIDs)+2 {
		t.Errorf("frames.tres holds %d entries, want %d", len(nowIDs), len(wasIDs)+2)
	}
}

// res path -> ext_resource id, in the order the file lists them.
func extIDsOf(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if !strings.HasPrefix(line, "[ext_resource ") {
			continue
		}
		if p := attrOf(line, "path"); p != "" {
			out[p] = attrOf(line, "id")
		}
	}
	if len(out) == 0 {
		t.Fatalf("no ext_resources in %s", path)
	}
	return out
}

// The manifest is load-bearing now, so losing it has to degrade to a rebuild
// rather than to output that points at files which are not there.
func TestMissingManifestRebuildsCleanly(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")

	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	frames := filepath.Join(out, "chars", "alice", "alice_frames.tres")
	for p := range extIDsOf(t, frames) {
		rel := strings.TrimPrefix(p, "res://out/")
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Errorf("frames.tres points at a missing file: %s", p)
		}
	}
}

// A clone with no cache recovers its layout from the .tres files already in
// the output tree, rather than reflowing every sheet on its first run. Growing
// the first layer is what makes this bite: a fresh pack would shift every
// layer after it.
func TestLayoutIsAdoptedFromDiskWithoutManifest(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	before := regionsIn(t, out)

	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}
	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "layer_00.png"),
		40, 40, []block{{x: 2, y: 2, w: 30, h: 30}})
	mustRun(t, dir, "--cli")

	grown := filepath.Join("env", "house", "layer_00.tres")
	after := regionsIn(t, out)
	for p, was := range before {
		if p == grown {
			continue
		}
		if after[p] != was {
			t.Errorf("%s reflowed from %v to %v without a manifest", p, was, after[p])
		}
	}
	if after[grown] == before[grown] {
		t.Fatal("the grown layer kept its old slot, so this proves nothing")
	}
}

// An adopted layout can be inconsistent in ways igor's own never is, so an
// overlap has to cost the folder its layout rather than its pixels.
func TestOverlappingAdoptedLayoutFallsBackToRepack(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")

	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}

	// Two layers now claim the same origin at their real sizes, so both look
	// keepable and they collide.
	for _, name := range []string{"layer_00.tres", "layer_01.tres"} {
		p := filepath.Join(out, "env", "house", name)
		body := readFile(t, p)
		r := regionOf(t, p)
		body = strings.Replace(body,
			fmt.Sprintf("region = Rect2(%d, %d, %d, %d)", r.x, r.y, r.w, r.h),
			fmt.Sprintf("region = Rect2(0, 0, %d, %d)", r.w, r.h), 1)
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustRun(t, dir, "--cli")

	regions := regionsIn(t, out)
	for a, ra := range regions {
		for b, rb := range regions {
			if a >= b || filepath.Dir(a) != filepath.Dir(b) {
				continue
			}
			if ra.intersects(rb) {
				t.Errorf("%v and %v overlap after the fallback (%s, %s)", ra, rb, a, b)
			}
		}
	}
}

// --repack is the only thing allowed to reflow a sheet, and it still must not
// break the output.
func TestRepackRunsClean(t *testing.T) {
	dir := layoutFixture(t)
	mustRun(t, dir, "--cli")
	mustRun(t, dir, "--cli", "--all", "--repack")

	frames := filepath.Join(dir, "out", "chars", "alice", "alice_frames.tres")
	for p := range extIDsOf(t, frames) {
		rel := strings.TrimPrefix(p, "res://out/")
		if _, err := os.Stat(filepath.Join(dir, "out", filepath.FromSlash(rel))); err != nil {
			t.Errorf("--repack left frames.tres pointing at %s", p)
		}
	}
}

// The folder an orphan sits in is usually clean by the time the human runs
// --clean, so the scan cannot be driven off the work queue.
func TestOrphanedTresRemovedFromCleanFolder(t *testing.T) {
	dir := layoutFixture(t)
	orphan := filepath.Join(dir, "out", "chars", "alice", "jump", "jump-03.tres")
	mustRun(t, dir, "--cli")

	if err := os.Remove(filepath.Join(dir, "in", "chars", "alice", "jump", "jump-03.png")); err != nil {
		t.Fatal(err)
	}

	r := mustRun(t, dir, "--cli")
	if !strings.Contains(r.stderr, "orphaned") {
		t.Errorf("a deleted frame should be reported as an orphan:\n%s", r.stderr)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("the orphan was removed without --clean: %v", err)
	}

	// The folder is unchanged by now, so nothing reaches the writers.
	r = mustRun(t, dir, "--cli", "--clean", "--force")
	if !strings.Contains(r.stdout+r.stderr, "Removed 1 of 1 orphaned") {
		t.Errorf("--clean did not remove the orphan:\n%s\n%s", r.stdout, r.stderr)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("%s survived --clean", orphan)
	}
}

// A folder that shrinks from four sheets to two leaves the other two behind.
func TestOrphanedBinsRemoved(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 32\nslice-size: 32\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	// Two of these never share a sheet, so each takes its own bin.
	for i := range 4 {
		writeBlocks(t, filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i)),
			32, 32, []block{{x: 0, y: 0, w: 24, h: 24}})
	}
	mustRun(t, dir, "--cli")

	bins := func() []string {
		t.Helper()
		found, err := filepath.Glob(filepath.Join(dir, "out", "env", "house", "house_*.png"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(found)
		return found
	}
	if got := bins(); len(got) != 4 {
		t.Fatalf("expected 4 bins, got %d: %v", len(got), got)
	}

	for _, i := range []int{2, 3} {
		if err := os.Remove(filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i))); err != nil {
			t.Fatal(err)
		}
	}

	// The shed sheets are held while their orphaned .tres still point at them,
	// since removing one out from under a resource is a Godot import error.
	mustRun(t, dir, "--cli")
	if got := bins(); len(got) != 4 {
		t.Errorf("a sheet was reclaimed while an orphan still referenced it: %v", got)
	}

	mustRun(t, dir, "--cli", "--clean", "--force")
	if got := bins(); len(got) != 2 {
		t.Errorf("expected 2 bins after shrinking, got %d: %v", len(got), got)
	}
}

// The done view gained the written/unchanged counters and the repack hints.
// Every other test shells out to the binary, so nothing else renders it.
func TestDoneViewRendersCountersAndHints(t *testing.T) {
	sesh.FilesWritten.Store(3)
	sesh.FilesUnchanged.Store(41)
	sesh.FoldersSkipped.Store(7)
	t.Cleanup(func() {
		sesh.FilesWritten.Store(0)
		sesh.FilesUnchanged.Store(0)
		sesh.FoldersSkipped.Store(0)
	})

	m := processModel{
		phase:       done,
		width:       100,
		folders:     []folder{{path: "env/house"}},
		repackHints: []string{"env/house: --repack would save 44% of its sheet area"},
	}

	view := m.View()
	for _, want := range []string{"3 files written", "41 unchanged", "7 unchanged folders skipped", "--repack would save 44%"} {
		if !strings.Contains(view, want) {
			t.Errorf("done view missing %q:\n%s", want, view)
		}
	}

	if line := m.summaryLine(); !strings.Contains(line, "3 files written, 41 unchanged") {
		t.Errorf("summary line reads %q", line)
	}
}

// --clean only deletes paths whose source is gone. Folders that survive keep
// their layout, so reaching for --clean to drop an orphan must not reflow the
// folder you happened to be editing.
func TestCleanDoesNotReflowSurvivingFolders(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	before := regionsIn(t, out)

	// Growing the first layer is what a reflow would show: a fresh pack shifts
	// every layer after it.
	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "layer_00.png"),
		40, 40, []block{{x: 2, y: 2, w: 30, h: 30}})
	mustRun(t, dir, "--cli", "--clean", "--force")

	grown := filepath.Join("env", "house", "layer_00.tres")
	after := regionsIn(t, out)
	for p, was := range before {
		if p == grown {
			continue
		}
		if after[p] != was {
			t.Errorf("--clean reflowed %s from %v to %v", p, was, after[p])
		}
	}
	if after[grown] == before[grown] {
		t.Fatal("the grown layer kept its old slot, so this proves nothing")
	}
}

// The repack hint tells the human to run --repack, so --repack on its own has
// to actually repack rather than skip every unchanged folder.
func TestRepackAloneRebuildsEverything(t *testing.T) {
	dir := layoutFixture(t)
	mustRun(t, dir, "--cli")

	r := mustRun(t, dir, "--cli", "--repack")
	if !strings.Contains(r.stdout, "0 folders skipped") {
		t.Errorf("--repack skipped unchanged folders, so it did nothing:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "3 folders,") {
		t.Errorf("--repack should have taken every folder:\n%s", r.stdout)
	}
}

// A folder whose source and output are both gone is visited by neither stale
// detection nor dirty detection, so its manifest entry used to live forever
// and put dangling paths into the character's frames resource.
func TestVanishedFolderLeavesNoDanglingFrames(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")

	if err := os.RemoveAll(filepath.Join(dir, "in", "chars", "alice", "run")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(out, "chars", "alice", "run")); err != nil {
		t.Fatal(err)
	}

	// Something has to make alice dirty, or the frames resource is never rewritten.
	writeBlocks(t, filepath.Join(dir, "in", "chars", "alice", "jump", "jump-02.png"),
		16, 16, []block{{x: 2, y: 2, w: 10, h: 10}})
	mustRun(t, dir, "--cli")

	frames := filepath.Join(out, "chars", "alice", "alice_frames.tres")
	for p := range extIDsOf(t, frames) {
		rel := strings.TrimPrefix(p, "res://out/")
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Errorf("frames.tres references a vanished animation: %s", p)
		}
	}
}

// A sprite whose own name ends in _frames must still be reportable as an
// orphan; only the character's frames resource is exempt.
func TestSpriteNamedFramesIsNotMistakenForAFramesResource(t *testing.T) {
	dir := projectWith(t, "  \"env/**\": { mode: env }\n")
	for _, name := range []string{"attack_frames.png", "roof.png"} {
		writeBlocks(t, filepath.Join(dir, "in", "env", "house", name), 40, 40,
			[]block{{x: 5, y: 5, w: 12, h: 12}})
	}
	mustRun(t, dir, "--cli")

	orphan := filepath.Join(dir, "out", "env", "house", "attack_frames.tres")
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("fixture never produced %s: %v", orphan, err)
	}
	if err := os.Remove(filepath.Join(dir, "in", "env", "house", "attack_frames.png")); err != nil {
		t.Fatal(err)
	}

	mustRun(t, dir, "--cli")
	r := mustRun(t, dir, "--cli", "--clean", "--force")
	if !strings.Contains(r.stdout+r.stderr, "Removed 1 of 1 orphaned") {
		t.Errorf("a sprite named *_frames was invisible to orphan cleanup:\n%s\n%s", r.stdout, r.stderr)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("%s survived --clean", orphan)
	}
}

// Inserting a frame shifts every later name by one. Carrying the uid across
// that shift would republish different art under the old identity, which looks
// like nothing happened. Section 8 asks for the loud failure instead.
func TestRenamedSpriteDoesNotInheritTheOldUID(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	second := filepath.Join(out, "chars", "alice", "run", "run_02.tres")
	wasUID := uidOf(t, second)
	if wasUID == "" {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}

	// Sorts between a and b, so it takes the run_02 name and pushes b to run_03.
	writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", "aa.png"), 16, 16)
	r := mustRun(t, dir, "--cli")

	if !strings.Contains(r.stderr, "renamed by position") {
		t.Errorf("the rename went unreported:\n%s", r.stderr)
	}
	if got := uidOf(t, second); got == wasUID {
		t.Errorf("run_02 now holds different art under the old uid %s", wasUID)
	}
	if got := uidOf(t, filepath.Join(out, "chars", "alice", "run", "run_01.tres")); got == "" {
		t.Error("run_01 did not change hands and should have kept its uid")
	}
}

// The uid on a resource's own header, or empty.
func uidOf(t *testing.T, path string) string {
	t.Helper()
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if strings.HasPrefix(line, "[gd_resource ") || strings.HasPrefix(line, "[gd_scene ") {
			return attrOf(line, "uid")
		}
	}
	return ""
}

// An interior bin keeps its index so later sheets are not renumbered, but the
// sheet itself should not linger once nothing points at it.
func TestEmptiedInteriorBinLeavesNoSheet(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 32\nslice-size: 32\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	for i := range 4 {
		writeBlocks(t, filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i)),
			32, 32, []block{{x: 0, y: 0, w: 24, h: 24}})
	}
	house := filepath.Join(dir, "out", "env", "house")
	mustRun(t, dir, "--cli")

	// The first two sheets empty out; the last two have to keep their indices.
	// Each emptied sheet is held while its orphaned .tres still points at it,
	// so --clean is what finally reclaims both together.
	for _, i := range []int{0, 1} {
		if err := os.Remove(filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i))); err != nil {
			t.Fatal(err)
		}
	}
	mustRun(t, dir, "--cli")
	mustRun(t, dir, "--cli", "--clean", "--force")

	for _, i := range []int{0, 1} {
		p := filepath.Join(house, fmt.Sprintf("house_%02d.png", i))
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived with nothing pointing at it", p)
		}
	}
	for _, i := range []int{2, 3} {
		name := fmt.Sprintf("layer_%02d.tres", i)
		body := readFile(t, filepath.Join(house, name))
		if want := fmt.Sprintf("house_%02d.png", i); !strings.Contains(body, want) {
			t.Errorf("%s was renumbered off %s:\n%s", name, want, body)
		}
	}
}

// Slice tiles are the one writer that was left on os.Create, so a project with
// oversized art rewrote every tile on each rebuild while reporting none.
func TestSliceTilesAreNotRewrittenWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 128\nslice-size: 16\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "backdrop.png"), 100, 60,
		[]block{{x: 5, y: 5, w: 80, h: 40}})

	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")

	tiles, err := filepath.Glob(filepath.Join(out, "env", "house", "backdrop", "*.png"))
	if err != nil || len(tiles) < 2 {
		t.Fatalf("fixture produced %d slice tiles, want several: %v", len(tiles), err)
	}

	before := snapshot(t, out)
	r := mustRun(t, dir, "--cli", "--all")

	if changed := rewritten(before, snapshot(t, out)); len(changed) > 0 {
		t.Errorf("a rebuild rewrote %d unchanged file(s): %v", len(changed), changed)
	}
	if !strings.Contains(r.stdout, "0 files written") {
		t.Errorf("summary should report nothing written:\n%s", r.stdout)
	}
}

// The uid guard leans on the manifest, which is empty on the first run after a
// format bump and on a clone that never committed the cache -- the very runs
// the on-disk adoption path exists to serve.
func TestRenamedSpriteDoesNotInheritUIDWithoutAManifest(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	second := filepath.Join(out, "chars", "alice", "run", "run_02.tres")
	wasUID := uidOf(t, second)
	if wasUID == "" {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}

	// No cache to compare names against, and a frame lands in the middle.
	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", "aa.png"), 16, 16)
	r := mustRun(t, dir, "--cli")

	if got := uidOf(t, second); got == wasUID {
		t.Errorf("run_02 holds different art under the old uid %s", wasUID)
	}
	if !strings.Contains(r.stderr, "reassigned rather than guessed") {
		t.Errorf("the reassignment went unreported:\n%s", r.stderr)
	}
}

// An unchanged tree with no cache must still keep its uids, or the format bump
// costs every reference in the project.
func TestUIDsSurviveAManifestlessRunOfUnchangedArt(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	second := filepath.Join(out, "chars", "alice", "run", "run_02.tres")
	wasUID := uidOf(t, second)
	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if got := uidOf(t, second); got != wasUID {
		t.Errorf("run_02 lost its uid on a cache-less rebuild: %q -> %q", wasUID, got)
	}
}

// An emptied interior bin loses its sheet but keeps its index, so sheet
// numbering has gaps and the orphan sweep cannot stop at the first miss.
func TestOrphanedSheetsAfterAGapAreStillRemoved(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 32\nslice-size: 32\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	for i := range 4 {
		writeBlocks(t, filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i)),
			32, 32, []block{{x: 0, y: 0, w: 24, h: 24}})
	}
	house := filepath.Join(dir, "out", "env", "house")
	src := filepath.Join(dir, "in", "env", "house")
	mustRun(t, dir, "--cli")

	// Punch a gap at sheet 1. The sheet is held while its orphaned .tres still
	// points at it, and --clean takes both.
	if err := os.Remove(filepath.Join(src, "layer_01.png")); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")
	mustRun(t, dir, "--cli", "--clean", "--force")
	if _, err := os.Stat(filepath.Join(house, "house_01.png")); !os.IsNotExist(err) {
		t.Fatal("the emptied interior sheet should be gone, so this proves nothing")
	}

	// Now shrink past the gap: everything from sheet 1 up has to go, which
	// means the sweep cannot stop at the index that is already missing.
	for _, n := range []string{"layer_02.png", "layer_03.png"} {
		if err := os.Remove(filepath.Join(src, n)); err != nil {
			t.Fatal(err)
		}
	}
	mustRun(t, dir, "--cli")
	mustRun(t, dir, "--cli", "--clean", "--force")

	for _, n := range []string{"house_02.png", "house_03.png"} {
		if _, err := os.Stat(filepath.Join(house, n)); !os.IsNotExist(err) {
			t.Errorf("%s survived behind a numbering gap", n)
		}
	}
	if _, err := os.Stat(filepath.Join(house, "house_00.png")); err != nil {
		t.Errorf("the surviving sheet was removed: %v", err)
	}
}

// A .tres is no use without the sheet it points at, and a sheet lost to a bad
// merge or a gitignore used to be invisible to dirty detection forever.
func TestMissingSpritesheetRebuildsTheFolder(t *testing.T) {
	dir := layoutFixture(t)
	sheet := filepath.Join(dir, "out", "chars", "alice", "jump", "jump_00.png")
	mustRun(t, dir, "--cli")

	if err := os.Remove(sheet); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if _, err := os.Stat(sheet); err != nil {
		t.Errorf("the folder was skipped and its sheet never came back: %v", err)
	}
}

// Nothing schedules a frames rewrite for an animation that leaves no trace:
// no source means no work, and no output means no prune.
func TestVanishedAnimationIsDroppedFromFramesWithNoDirtySibling(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")

	frames := filepath.Join(out, "chars", "alice", "alice_frames.tres")
	if !strings.Contains(readFile(t, frames), "/run/") {
		t.Fatal("fixture never referenced the run animation, so this proves nothing")
	}

	// Both sides vanish, and every other folder stays clean.
	if err := os.RemoveAll(filepath.Join(dir, "in", "chars", "alice", "run")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(out, "chars", "alice", "run")); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if body := readFile(t, frames); strings.Contains(body, "/run/") {
		t.Errorf("frames.tres still references the vanished animation:\n%s", body)
	}
	for p := range extIDsOf(t, frames) {
		rel := strings.TrimPrefix(p, "res://out/")
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel))); err != nil {
			t.Errorf("frames.tres references a missing file: %s", p)
		}
	}
}

// writeTres drops a uid so a stale reference breaks loudly. The frames
// resource must not quietly put it back.
func TestFramesDoesNotReasserAResetUID(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	second := filepath.Join(out, "chars", "alice", "run", "run_02.tres")
	staleUID := uidOf(t, second)
	if staleUID == "" {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}

	writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", "aa.png"), 16, 16)
	mustRun(t, dir, "--cli")

	if got := uidOf(t, second); got == staleUID {
		t.Fatal("the .tres kept the old uid, so this proves nothing")
	}
	frames := readFile(t, filepath.Join(out, "chars", "alice", "alice_frames.tres"))
	if strings.Contains(frames, staleUID) {
		t.Errorf("frames.tres reasserted the uid igor just invalidated:\n%s", frames)
	}
}

// The TUI renders its log box only while trimming, so a warning raised during
// writing has to survive to the end of the run.
func TestWritePhaseWarningsAreReported(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")

	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", "aa.png"), 16, 16)

	r := mustRun(t, dir, "--cli")
	if n := strings.Count(r.stderr, "reassigned rather than guessed"); n != 1 {
		t.Errorf("warning appeared %d times, want exactly 1:\n%s", n, r.stderr)
	}
}

// In TUI mode the warning is not printed when it is raised, so it has to be
// held and replayed once the program has quit.
func TestWarningsAreHeldForTheEndOfTheRun(t *testing.T) {
	wasCLI := sesh.CLI
	t.Cleanup(func() { sesh.CLI = wasCLI; sesh.takeWarnings() })

	// Raised during writing, where the TUI log box is no longer rendered.
	sesh.CLI = true
	warnLate("uids here are being reassigned rather than guessed")

	sesh.CLI = false
	out := captureStdout(t, reportWarnings)
	if !strings.Contains(out, "reassigned rather than guessed") {
		t.Errorf("the warning was dropped instead of replayed, got: %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	was := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = was
	w.Close()

	var b strings.Builder
	if _, err := io.Copy(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// An orphan left by an earlier build still holds its old uid. A new frame that
// lands on that name must not inherit it.
func TestResurrectedNameDoesNotInheritAnOrphansUID(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	src := filepath.Join(dir, "in", "chars", "alice", "run")
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		writePNG(t, filepath.Join(src, name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	third := filepath.Join(out, "chars", "alice", "run", "run_03.tres")
	orphanUID := uidOf(t, third)
	if orphanUID == "" {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}

	// c goes, leaving run_03.tres behind as an orphan (only --clean removes it).
	if err := os.Remove(filepath.Join(src, "c.png")); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")
	if _, err := os.Stat(third); err != nil {
		t.Fatalf("expected run_03.tres to linger as an orphan: %v", err)
	}

	// A different image now takes the vacated name.
	writePNG(t, filepath.Join(src, "d.png"), 20, 20)
	mustRun(t, dir, "--cli")

	if got := uidOf(t, third); got == orphanUID {
		t.Errorf("run_03 inherited a stale orphan's uid %s", orphanUID)
	}
}

// Positional names carried without a build record are taken on trust, and a
// delete plus an add in one run is the case that trust cannot survive. Nothing
// on disk can reveal it, so igor has to say that it is assuming.
func TestCacheLessPositionalCarryIsAnnounced(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	src := filepath.Join(dir, "in", "chars", "alice", "run")
	for _, name := range []string{"b.png", "c.png", "d.png"} {
		writePNG(t, filepath.Join(src, name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	second := filepath.Join(out, "chars", "alice", "run", "run_02.tres")
	wasUID := uidOf(t, second)
	if wasUID == "" {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}

	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}
	// d out, a in: still three sprites, but every name shifts by one.
	if err := os.Remove(filepath.Join(src, "d.png")); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(src, "a.png"), 16, 16)
	r := mustRun(t, dir, "--cli")

	// The uid is kept, because nothing on disk distinguishes this from an
	// untouched folder. What igor owes the user is to say so.
	if got := uidOf(t, second); got != wasUID {
		t.Errorf("run_02 dropped its uid where the names looked unchanged: %q -> %q", wasUID, got)
	}
	if !strings.Contains(r.stderr, "kept on the assumption") {
		t.Errorf("a cache-less carry went unannounced:\n%s", r.stderr)
	}
}

// Packing supersedes sheets, but the .tres pointing at them are only rewritten
// later, so the removal has to wait for a run that finished writing.
func TestStaleSheetsSurviveAFailedRun(t *testing.T) {
	wasDest, wasCLI := prj.Destination, sesh.CLI
	prj.Destination = t.TempDir()
	sesh.CLI = true
	t.Cleanup(func() {
		prj.Destination, sesh.CLI = wasDest, wasCLI
		sesh.takeStaleSheets()
	})

	sheet := filepath.Join(prj.Destination, "house_01.png")
	write := func() {
		if err := os.WriteFile(sheet, []byte("sheet"), 0644); err != nil {
			t.Fatal(err)
		}
		sesh.addStaleSheet(sheet)
	}

	write()
	m := processModel{phase: writing, exceptions: []exception{{code: systemError, msg: "something failed"}}}
	m.Update(writingCompleteMsg{})
	if _, err := os.Stat(sheet); err != nil {
		t.Errorf("a failed run removed a sheet its resources may still need: %v", err)
	}

	write()
	clean := processModel{phase: writing}
	clean.Update(writingCompleteMsg{})
	if _, err := os.Stat(sheet); !os.IsNotExist(err) {
		t.Errorf("a clean run left the superseded sheet behind")
	}
}

// Trimming-phase warnings are already shown live, so replaying them after the
// TUI exits would duplicate them, unbounded.
func TestOnlyLateWarningsAreReplayed(t *testing.T) {
	wasCLI := sesh.CLI
	t.Cleanup(func() { sesh.CLI = wasCLI; sesh.takeWarnings() })

	sesh.CLI = true
	warn("this one was shown live while trimming")
	warnLate("this one was raised during writing")

	sesh.CLI = false
	out := captureStdout(t, reportWarnings)
	if strings.Contains(out, "shown live while trimming") {
		t.Errorf("a warning the log box already showed was replayed:\n%s", out)
	}
	if !strings.Contains(out, "raised during writing") {
		t.Errorf("the late warning was dropped:\n%s", out)
	}
}

// res path -> the uid an ext_resource line attaches to it.
func extUIDsOf(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(readFile(t, path), "\n") {
		if !strings.HasPrefix(line, "[ext_resource ") {
			continue
		}
		if p := attrOf(line, "path"); p != "" {
			out[p] = attrOf(line, "uid")
		}
	}
	return out
}

// A sprite rebuilt from nothing has no uid until Godot imports it, so the
// frames resource must not keep declaring the one it used to have.
func TestFramesDropsTheUIDOfARebuiltSprite(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	sprite := filepath.Join(out, "chars", "alice", "jump", "jump-02.tres")
	frames := filepath.Join(out, "chars", "alice", "alice_frames.tres")
	resPath := "res://out/chars/alice/jump/jump-02.tres"
	if extUIDsOf(t, frames)[resPath] == "" {
		t.Fatal("fixture never stamped a reference uid, so this proves nothing")
	}

	if err := os.Remove(sprite); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if uidOf(t, sprite) != "" {
		t.Fatal("the rebuilt sprite somehow kept a uid, so this proves nothing")
	}
	if got := extUIDsOf(t, frames)[resPath]; got != "" {
		t.Errorf("frames.tres still declares uid %s for a sprite that has none", got)
	}
}

// The character frames resource is manifest-derived output like any other, so
// losing it has to make something dirty.
func TestMissingFramesResourceIsRegenerated(t *testing.T) {
	dir := layoutFixture(t)
	frames := filepath.Join(dir, "out", "chars", "alice", "alice_frames.tres")
	mustRun(t, dir, "--cli")

	if err := os.Remove(frames); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if _, err := os.Stat(frames); err != nil {
		t.Errorf("nothing noticed the frames resource had gone: %v", err)
	}
}

// A first build is not assuming anything about identity, and should not say
// that it is.
func TestFirstBuildDoesNotWarnAboutTheCache(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 16, 16)
	}

	r := mustRun(t, dir, "--cli")
	if strings.Contains(r.stderr, "no build cache") {
		t.Errorf("a first build warned about carrying identity it never had:\n%s", r.stderr)
	}
}

// An animation skipped because its output is missing is not an animation that
// is gone, and must not take the character's frames resource with it.
func TestFramesSurvivesAnimationsThatAreMerelyMissingOutput(t *testing.T) {
	dir := layoutFixture(t)
	out := filepath.Join(dir, "out")
	frames := filepath.Join(out, "chars", "alice", "alice_frames.tres")
	mustRun(t, dir, "--cli")

	// jump vanishes outright, which is what schedules a frames rewrite.
	if err := os.RemoveAll(filepath.Join(dir, "in", "chars", "alice", "jump")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(out, "chars", "alice", "jump")); err != nil {
		t.Fatal(err)
	}

	// run keeps its manifest entry and its folder, but loses its textures, so
	// it is skipped rather than pruned.
	if err := os.RemoveAll(filepath.Join(dir, "in", "chars", "alice", "run")); err != nil {
		t.Fatal(err)
	}
	tres, err := filepath.Glob(filepath.Join(out, "chars", "alice", "run", "*.tres"))
	if err != nil || len(tres) == 0 {
		t.Fatalf("expected run to have textures: %v", err)
	}
	for _, p := range tres {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}

	mustRun(t, dir, "--cli")

	if _, err := os.Stat(frames); err != nil {
		t.Errorf("the frames resource was deleted over animations that still have records: %v", err)
	}
}

// Rule resolution is now consulted independently by dirty detection, orphan
// scanning and the frames writer, so an unstable answer means they disagree
// about the same folder. Ranging a Go map gave a different answer per call.
func TestOverlappingRulesResolveToTheNarrowest(t *testing.T) {
	wasRules := prj.Rules
	t.Cleanup(func() { prj.Rules = wasRules })

	prj.Rules = map[string]ruleConfig{
		"chars/**":        {Mode: "character", RenameLayers: true},
		"chars/hero/idle": {Mode: "item"},
	}

	for range 20 {
		if got := getFolderConfig("chars/hero/idle"); got.typ != FolderTypeItem {
			t.Fatalf("the narrower rule lost: got %q", got.typ)
		}
		if got := getFolderConfig("chars/hero/walk"); got.typ != FolderTypeCharacter {
			t.Fatalf("the broad rule should still cover siblings: got %q", got.typ)
		}
	}
}

// An env package's scene and a sliced image's scene are manifest-derived
// output as much as any .tres, so losing one has to make the folder dirty.
func TestMissingEnvSceneIsRegenerated(t *testing.T) {
	dir := layoutFixture(t)
	scene := filepath.Join(dir, "out", "env", "house", "house.tscn")
	mustRun(t, dir, "--cli")

	if err := os.Remove(scene); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if _, err := os.Stat(scene); err != nil {
		t.Errorf("nothing noticed the env scene had gone: %v", err)
	}
}

func TestMissingSliceSceneIsRegenerated(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 128\nslice-size: 16\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "backdrop.png"), 100, 60,
		[]block{{x: 5, y: 5, w: 80, h: 40}})

	scene := filepath.Join(dir, "out", "env", "house", "backdrop", "backdrop.tscn")
	mustRun(t, dir, "--cli")
	if _, err := os.Stat(scene); err != nil {
		t.Fatalf("fixture never produced a slice scene: %v", err)
	}

	if err := os.Remove(scene); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if _, err := os.Stat(scene); err != nil {
		t.Errorf("nothing noticed the slice scene had gone: %v", err)
	}
}

// --clean --force deletes without prompting, so the orphan list must hold only
// files igor can recognise as its own.
func TestHandAuthoredResourcesAreNotTreatedAsOrphans(t *testing.T) {
	dir := layoutFixture(t)
	house := filepath.Join(dir, "out", "env", "house")
	mustRun(t, dir, "--cli")

	mine := filepath.Join(house, "wind_material.tres")
	body := "[gd_resource type=\"ShaderMaterial\" format=3]\n\n[resource]\nresource_name = \"wind\"\n"
	if err := os.WriteFile(mine, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	r := mustRun(t, dir, "--cli", "--clean", "--force")
	if strings.Contains(r.stdout+r.stderr, "wind_material") {
		t.Errorf("a hand-authored resource was listed as an orphan:\n%s\n%s", r.stdout, r.stderr)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("--clean --force deleted a resource igor did not write: %v", err)
	}
	if got := readFile(t, mine); got != body {
		t.Errorf("the resource was rewritten:\n%s", got)
	}
}

// A regenerated slice scene has no uid until Godot imports it, so the env
// scene that instances it must stop declaring the old one.
func TestEnvSceneDropsTheUIDOfARebuiltSliceScene(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 128\nslice-size: 16\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	writeBlocks(t, filepath.Join(dir, "in", "env", "house", "backdrop.png"), 100, 60,
		[]block{{x: 5, y: 5, w: 80, h: 40}})
	writePNG(t, filepath.Join(dir, "in", "env", "house", "roof.png"), 10, 10)

	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	scene := filepath.Join(out, "env", "house", "backdrop", "backdrop.tscn")
	house := filepath.Join(out, "env", "house", "house.tscn")
	resPath := "res://out/env/house/backdrop/backdrop.tscn"
	if extUIDsOf(t, house)[resPath] == "" {
		t.Fatal("fixture never stamped a reference uid, so this proves nothing")
	}

	if err := os.Remove(scene); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")

	if uidOf(t, scene) != "" {
		t.Fatal("the rebuilt slice scene somehow kept a uid, so this proves nothing")
	}
	if got := extUIDsOf(t, house)[resPath]; got != "" {
		t.Errorf("house.tscn still declares uid %s for a scene that has none", got)
	}
}

// Only igor's own sprites are evidence about what the output names mean. A
// resource someone put in the folder must not cost it every uid it has.
func TestHandAuthoredResourceDoesNotCostTheFolderItsUIDs(t *testing.T) {
	dir := projectWith(t, "  \"chars/**\": { mode: character, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "chars", "alice", "run", name), 16, 16)
	}
	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	second := filepath.Join(out, "chars", "alice", "run", "run_02.tres")
	wasUID := uidOf(t, second)
	if wasUID == "" {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}

	handmade := filepath.Join(out, "chars", "alice", "run", "handmade.tres")
	body := "[gd_resource type=\"ShaderMaterial\" format=3]\n\n[resource]\n"
	if err := os.WriteFile(handmade, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, manifestFilename)); err != nil {
		t.Fatal(err)
	}

	r := mustRun(t, dir, "--cli")
	if got := uidOf(t, second); got != wasUID {
		t.Errorf("a hand-authored resource cost run_02 its uid: %q -> %q", wasUID, got)
	}
	if strings.Contains(r.stderr, "sprite names have changed") {
		t.Errorf("igor claimed names changed when none did:\n%s", r.stderr)
	}
}

// A --clean run prompts twice. bufio reads ahead, so a second reader built for
// the second prompt finds the piped answers already consumed.
func TestTwoPromptsCanBothBeAnswered(t *testing.T) {
	wasStdin, wasForce, wasCLI := os.Stdin, sesh.Force, sesh.CLI
	t.Cleanup(func() {
		os.Stdin, sesh.Force, sesh.CLI = wasStdin, wasForce, wasCLI
		stdinReader = nil
	})
	sesh.Force, sesh.CLI = false, true

	answers := filepath.Join(t.TempDir(), "answers")
	if err := os.WriteFile(answers, []byte("y\ny\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(answers)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	os.Stdin = f
	stdinReader = nil

	for i, want := range []bool{true, true} {
		got, code := confirmOrFail("Delete them?")
		if code != 0 {
			t.Fatalf("prompt %d failed with exit %d; the piped answer was lost", i+1, code)
		}
		if got != want {
			t.Errorf("prompt %d read %v, want %v", i+1, got, want)
		}
	}
}

// --clean --force deletes without a prompt, so provenance has to come from
// igor's own record. A hand-authored AtlasTexture cropping igor's spritesheet
// is shaped exactly like a stranded sprite.
func TestHandAuthoredAtlasTextureSurvivesClean(t *testing.T) {
	dir := layoutFixture(t)
	house := filepath.Join(dir, "out", "env", "house")
	mustRun(t, dir, "--cli")

	// Shaped like igor's output in every respect: AtlasTexture, one
	// ext_resource, a region, and this folder's own sheet.
	mine := filepath.Join(house, "my_crop.tres")
	body := "[gd_resource type=\"AtlasTexture\" format=3 uid=\"uid://handmade1\"]\n\n" +
		"[ext_resource type=\"Texture2D\" path=\"res://out/env/house/house_00.png\" id=\"1\"]\n\n" +
		"[resource]\natlas = ExtResource(\"1\")\nregion = Rect2(0, 0, 8, 8)\n"
	if err := os.WriteFile(mine, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	r := mustRun(t, dir, "--cli", "--clean", "--force")
	if strings.Contains(r.stdout+r.stderr, "my_crop") {
		t.Errorf("a hand-authored AtlasTexture was listed as an orphan:\n%s\n%s", r.stdout, r.stderr)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("--clean --force deleted a resource igor never wrote: %v", err)
	}

	// And a real orphan in the same folder still goes.
	if err := os.Remove(filepath.Join(dir, "in", "env", "house", "layer_03.png")); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")
	mustRun(t, dir, "--cli", "--clean", "--force")
	if _, err := os.Stat(filepath.Join(house, "layer_03.tres")); !os.IsNotExist(err) {
		t.Error("the real orphan survived, so the record is too strict")
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("the hand-authored resource went with it: %v", err)
	}
}

// Sheet cleanup matches on filename shape, so it must only reclaim indices
// igor recorded writing.
func TestUserPNGNamedLikeASheetSurvives(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 32\nslice-size: 32\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		writeBlocks(t, filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i)),
			32, 32, []block{{x: 0, y: 0, w: 24, h: 24}})
	}
	mustRun(t, dir, "--cli")

	// Never written by igor, but named exactly like one of its sheets.
	mine := filepath.Join(dir, "out", "env", "house", "house_07.png")
	writeBlocks(t, mine, 8, 8, []block{{x: 0, y: 0, w: 8, h: 8}})

	if err := os.Remove(filepath.Join(dir, "in", "env", "house", "layer_01.png")); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")
	mustRun(t, dir, "--cli", "--clean", "--force")

	if _, err := os.Stat(mine); err != nil {
		t.Errorf("a user PNG named like a sheet was swept up: %v", err)
	}
}

// Content that exactly fills half a sheet fits that half, and rendering the
// larger size costs four times the texture memory.
func TestSheetDimHalvesAtExactPowersOfTwo(t *testing.T) {
	was := prj.SpritesheetSize
	prj.SpritesheetSize = 4096
	t.Cleanup(func() { prj.SpritesheetSize = was })

	for _, tc := range []struct{ extent, want int }{
		{4096, 4096},
		{2049, 4096},
		{2048, 2048},
		{1025, 2048},
		{1024, 1024},
		{1, 1},
		{0, 1},
	} {
		if got := sheetDim(tc.extent); got != tc.want {
			t.Errorf("sheetDim(%d) = %d, want %d", tc.extent, got, tc.want)
		}
	}
}

// Specificity is about the pattern's shape, not its length.
func TestRulePrecedencePrefersFewerWildcards(t *testing.T) {
	wasRules := prj.Rules
	t.Cleanup(func() { prj.Rules = wasRules })

	// Both patterns match. Same segment count, same length, and the wildcard
	// one sorts first alphabetically, so only counting wildcards separates
	// them -- every other tie-break picks the wrong rule.
	prj.Rules = map[string]ruleConfig{
		"chars/*ero/props": {Mode: "env"},
		"chars/hero/props": {Mode: "item"},
	}

	for range 20 {
		if got := getFolderConfig("chars/hero/props"); got.typ != FolderTypeItem {
			t.Fatalf("the wildcard-free rule lost: got %q", got.typ)
		}
	}
}

// A divided image's pieces are named by reading-order position, and their
// slotKey carries that same positional name -- so the owner comparison that
// protects renameLayers compares a key to itself here and always agrees.
func TestInsertedBlobDoesNotInheritAPiecesUID(t *testing.T) {
	dir := projectWith(t, "  \"items/**\": { mode: item, divideAbove: 10 }\n")
	art := filepath.Join(dir, "in", "items", "props", "sheet.png")
	writeBlocks(t, art, 120, 40, []block{
		{x: 5, y: 5, w: 12, h: 12},
		{x: 90, y: 5, w: 14, h: 14},
	})

	out := filepath.Join(dir, "out")
	mustRun(t, dir, "--cli")
	stampUIDs(t, out)

	second := filepath.Join(out, "items", "props", "sheet_02.tres")
	wasUID := uidOf(t, second)
	if wasUID == "" {
		t.Fatal("fixture never stamped a uid, so this proves nothing")
	}
	if got := regionOf(t, second); got.w != 14 {
		t.Fatalf("sheet_02 should be the 14-wide blob, got %dx%d", got.w, got.h)
	}

	// A blob in the middle pushes the right-hand one onto sheet_03.
	writeBlocks(t, art, 120, 40, []block{
		{x: 5, y: 5, w: 12, h: 12},
		{x: 50, y: 5, w: 13, h: 13},
		{x: 90, y: 5, w: 14, h: 14},
	})
	r := mustRun(t, dir, "--cli")

	if got := regionOf(t, second); got.w != 13 {
		t.Fatalf("sheet_02 should now be the new middle blob, got %dx%d", got.w, got.h)
	}
	if got := uidOf(t, second); got == wasUID {
		t.Errorf("sheet_02 describes different art but kept uid %s", wasUID)
	}
	if !strings.Contains(r.stderr, "renumbering") {
		t.Errorf("the piece renumbering went unreported:\n%s", r.stderr)
	}
}

// An env package that renames its layers has to reference the names that were
// actually written.
func TestEnvSceneReferencesRenamedLayers(t *testing.T) {
	dir := projectWith(t, "  \"env/**\": { mode: env, renameLayers: true }\n")
	for _, name := range []string{"a.png", "b.png"} {
		writePNG(t, filepath.Join(dir, "in", "env", "stuff", name), 16, 16)
	}
	mustRun(t, dir, "--cli")

	stuff := filepath.Join(dir, "out", "env", "stuff")
	scene := filepath.Join(stuff, "stuff.tscn")
	for p := range extIDsOf(t, scene) {
		rel := strings.TrimPrefix(p, "res://out/")
		if _, err := os.Stat(filepath.Join(dir, "out", filepath.FromSlash(rel))); err != nil {
			t.Errorf("the scene references a file that was never written: %s", p)
		}
	}
}

// A run that aborts before writing leaves the cache untouched, so the sheets a
// folder shed are still attributable once the fault is gone. This covers the
// abort path; the unit test below covers the one where the cache is rewritten
// with a folder marked failed.
func TestSupersededSheetsSurviveAnAbortedRun(t *testing.T) {
	dir := t.TempDir()
	cfg := "destination: ./out\nsource: ./in\nspritesheet-size: 32\nslice-size: 32\nres-prefix: out/\nrules:\n" +
		"  \"env/**\": { mode: env }\n  \"chars/**\": { mode: character }\n"
	if err := os.WriteFile(filepath.Join(dir, "igor.yml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		writeBlocks(t, filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i)),
			32, 32, []block{{x: 0, y: 0, w: 24, h: 24}})
	}
	house := filepath.Join(dir, "out", "env", "house")
	mustRun(t, dir, "--cli")

	// house sheds two sheets, and a character frame too large to pack fails
	// the run before any of it is written.
	for _, i := range []int{1, 2} {
		if err := os.Remove(filepath.Join(dir, "in", "env", "house", fmt.Sprintf("layer_%02d.png", i))); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(dir, "in", "chars", "bob", "idle", "huge.png"), 200, 200)
	if r := runIgor(t, dir, "--cli"); r.code == 0 {
		t.Fatalf("the oversized frame should have failed the run:\n%s", r.stderr)
	}

	// With the fault gone, the shed sheets must still be reclaimable.
	if err := os.RemoveAll(filepath.Join(dir, "in", "chars")); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "--cli")
	mustRun(t, dir, "--cli", "--clean", "--force")

	for _, n := range []string{"house_01.png", "house_02.png"} {
		if _, err := os.Stat(filepath.Join(house, n)); !os.IsNotExist(err) {
			t.Errorf("%s could never be reclaimed after a failed run", n)
		}
	}
}

// A folder that failed has its digest dropped so it rebuilds, but dropping the
// whole entry would also drop the record of how many sheets igor wrote there,
// and nothing could ever attribute the ones it shed.
func TestFailedFolderKeepsItsSheetRecord(t *testing.T) {
	wasDest, wasPrior, wasDigests := prj.Destination, priorManifest, runDigests
	prj.Destination = t.TempDir()
	runDigests = map[string]string{"env/house": "fresh"}
	t.Cleanup(func() {
		prj.Destination, priorManifest, runDigests = wasDest, wasPrior, wasDigests
	})

	before := emptyManifest()
	before.Folders["env/house"] = folderEntry{
		Digest:  "stale",
		Bins:    3,
		Orphans: []string{"layer_09"},
	}
	if err := before.save(); err != nil {
		t.Fatal(err)
	}
	priorManifest = before

	failing := imageFile{path: "env/house", filename: "layer_00.png"}
	mf, ok := buildManifest(
		[]folder{{path: "env/house"}},
		nil,
		[]exception{{code: systemError, msg: "could not write", file: &failing}},
	)
	if !ok {
		t.Fatal("an attributable failure should still produce a manifest")
	}

	got := mf.Folders["env/house"]
	if got.Bins != 3 {
		t.Errorf("the sheet count was lost: Bins = %d, want 3", got.Bins)
	}
	if len(got.Orphans) != 1 || got.Orphans[0] != "layer_09" {
		t.Errorf("the orphan record was lost: %v", got.Orphans)
	}
	if got.Digest == runDigests["env/house"] {
		t.Error("a failed folder must not be recorded as clean")
	}
}

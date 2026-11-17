package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/png"
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
// one added to the SpriteFrames resource that igor leaves without.
func stampUIDs(t *testing.T, root string) {
	t.Helper()
	n := 0
	nextUID := func() string {
		n++
		return fmt.Sprintf("uid://igortest%05d", n)
	}

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ext := filepath.Ext(p); d.IsDir() || (ext != ".tres" && ext != ".tscn") {
			return nil
		}

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
				lines[i] = strings.TrimSuffix(line, "]") + fmt.Sprintf(" uid=%q]", nextUID())
			case strings.HasPrefix(line, "[ext_resource "):
				lines[i] = strings.Replace(line, " path=", fmt.Sprintf(" uid=%q path=", nextUID()), 1)
			}
		}
		return os.WriteFile(p, []byte(strings.Join(lines, "\n")), 0644)
	})
	if err != nil {
		t.Fatal(err)
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
	if !strings.Contains(r.stdout+r.stderr, "Removed 1 orphaned") {
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
	mustRun(t, dir, "--cli")

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

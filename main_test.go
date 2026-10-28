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

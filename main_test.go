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

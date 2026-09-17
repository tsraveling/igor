package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	out, err := exec.Command("go", "build", "-o", binPath, ".").CombinedOutput()
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
	r := runIgor(t, fixture(t, true), "--cli")

	if r.code != 1 {
		t.Fatalf("exit %d, want 1\nstdout:\n%s\nstderr:\n%s", r.code, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "[TOO LARGE]") {
		t.Errorf("expected the exception on stderr, got:\n%s", r.stderr)
	}
	if strings.Contains(r.stdout, "[TOO LARGE]") {
		t.Errorf("exception leaked onto stdout:\n%s", r.stdout)
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

func TestConflictingFlags(t *testing.T) {
	dir := fixture(t, false)

	for _, args := range [][]string{{"--cli", "--tui"}, {"--quiet", "--verbose"}} {
		r := runIgor(t, dir, args...)
		if r.code == 0 {
			t.Errorf("%v should be rejected", args)
		}
	}
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// Output tiers for CLI mode.
type outputTier int

const (
	tierQuiet outputTier = iota
	tierNormal
	tierVerbose
)

func tier() outputTier {
	switch {
	case sesh.Quiet:
		return tierQuiet
	case sesh.Verbose:
		return tierVerbose
	default:
		return tierNormal
	}
}

// Reads the version stamped in by the go toolchain. Local builds have no
// version, so they report "dev".
func buildVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(bi.Main.Version, "v")
}

// All CLI output funnels through here, so a future --json only has to change
// these four.
func cliLine(min outputTier, line string) {
	if !sesh.CLI || tier() < min {
		return
	}
	fmt.Fprintln(os.Stdout, line)
}

func emitVersion() {
	cliLine(tierNormal, "igor "+buildVersion())
}

func emitPhase(p phase, format string, args ...any) {
	cliLine(tierNormal, fmt.Sprintf("[%s] %s", strings.ToLower(phaseNames[p]), fmt.Sprintf(format, args...)))
}

// Per-image and per-slice noise, shown under --verbose only.
func emitDetail(p phase, format string, args ...any) {
	cliLine(tierVerbose, fmt.Sprintf("[%s] %s", strings.ToLower(phaseNames[p]), fmt.Sprintf(format, args...)))
}

// Exceptions go to stderr at every tier, one line each so they stay greppable.
func emitException(exc exception) {
	if !sesh.CLI {
		return
	}
	line := fmt.Sprintf("[%s] %s", exceptionCodeName(exc.code), exc.msg)
	if exc.file != nil {
		line += "\t" + filepath.Join(exc.file.path, exc.file.filename)
	}
	fmt.Fprintln(os.Stderr, line)
}

// Non-fatal warnings share stderr with exceptions, since neither belongs in a
// piped build log's stdout.
func emitWarning(msg string) {
	if !sesh.CLI {
		return
	}
	fmt.Fprintln(os.Stderr, "[WARNING] "+msg)
}

func emitSummary(line string) {
	cliLine(tierQuiet, line)
}

// Prints an error and returns the exit code, so callers can bail before
// touching the filesystem.
func fatal(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	return 1
}

// Package matrix runs vk-gen against a set of pinned Vulkan registry files and checks that each one produces a Go
// package that builds. Versions with a golden copy in testdata/golden are also diffed against it.
//
// For each registry file testdata/registry/vk-<version>.xml, the stages are:
//
//	generate    vk-gen exits 0, logs no error-level messages, and produces identical output on a second run
//	gogenerate  `go generate` (stringer) succeeds
//	build       `go build` of the generated package
//	vet         `go vet` of the generated package
//	test        `go test` of the generated package (the static tests, which use a fake Vulkan library)
//	golden      the output, including the stringer files, matches testdata/golden/<version>
//
// Any vk-<version>.xml dropped into testdata/registry is picked up automatically. Without a golden directory it gets
// the first five stages only. To record a golden, run
//
//	go test -count=1 ./matrix -update
//
// which writes testdata/golden/<version> for every version that passes all of the stages.
//
// testdata/excluded_static_tests.json lists static test files that are removed from a version's output before the vet and
// test stages because the version predates an API the test uses (for example, loader_test.go needs VK_EXT_debug_utils).
//
// The generated package uses cgo and is built for the host GOOS only. The tests need goimports and stringer; set
// VK_MATRIX_REQUIRE_TOOLS=1 (or CI) to fail, rather than skip, when they are missing.
package matrix

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/golden for every version that passes all stages")

const (
	registryDir = "testdata/registry"
	goldenDir   = "testdata/golden"
	staticDir   = "../_static_include"
)

// Output that is not generated, and therefore not part of a golden copy: the module files written by the test, and the
// files copied from _static_include (see staticNames).
var testModuleFiles = map[string]bool{"go.mod": true, "go.sum": true}

// excludedTests lists static test files that are removed from a version's output before the vet and test stages, because
// they use an API that the version does not have.
type excludedTests struct {
	Files  []string `json:"files"`
	Reason string   `json:"reason"`
}

var (
	buildOnce sync.Once
	genPath   string
	buildErr  error
)

// TestMain builds vk-gen once. It is built as a binary, rather than called in-process, because vk-gen parses its flags in
// init() and exits through logrus.Fatal.
func TestMain(m *testing.M) {
	flag.Parse()

	dir, err := os.MkdirTemp("", "vk-gen-matrix")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	genPath = filepath.Join(dir, "vk-gen")
	if runtime.GOOS == "windows" {
		genPath += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", genPath, "..").CombinedOutput(); err != nil {
		buildErr = fmt.Errorf("building vk-gen: %v\n%s", err, out)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestRegistryChecksums(t *testing.T) {
	sums, err := os.ReadFile(filepath.Join(registryDir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
		var want, name string
		if _, err := fmt.Sscanf(line, "%s %s", &want, &name); err != nil {
			t.Fatalf("bad SHA256SUMS line %q", line)
		}
		data, err := os.ReadFile(filepath.Join(registryDir, name))
		if err != nil {
			t.Error(err)
			continue
		}
		got := sha256.Sum256(data)
		if hex.EncodeToString(got[:]) != want {
			t.Errorf("%s does not match SHA256SUMS; the pinned registry files must not change (see fetch.sh)", name)
		}
	}
}

func TestRegistryMatrix(t *testing.T) {
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	requireTool(t, "goimports")
	requireTool(t, "stringer")
	requireTool(t, "go")

	files, err := filepath.Glob(filepath.Join(registryDir, "vk-*.xml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no registry files found in %s (err %v)", registryDir, err)
	}

	statics := staticNames(t)

	excluded := map[string]excludedTests{}
	if data, err := os.ReadFile("testdata/excluded_static_tests.json"); err != nil {
		t.Fatal(err)
	} else if err := json.Unmarshal(data, &excluded); err != nil {
		t.Fatalf("excluded_static_tests.json: %v", err)
	}

	for _, f := range files {
		version := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "vk-"), ".xml")
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			runVersion(t, version, excluded[version], statics)
		})
	}

	for version, ex := range excluded {
		if _, err := os.Stat(filepath.Join(registryDir, "vk-"+version+".xml")); err != nil {
			t.Errorf("excluded_static_tests.json lists %s, which has no registry file", version)
		}
		for _, name := range ex.Files {
			if !statics[name] || !strings.HasSuffix(name, "_test.go") {
				t.Errorf("excluded_static_tests.json: %s for %s is not a test file in _static_include", name, version)
			}
		}
	}
}

func runVersion(t *testing.T, version string, excluded excludedTests, statics map[string]bool) {
	out := t.TempDir()
	env := toolEnv()

	stages := []struct {
		name string
		fn   func() error
	}{
		{"generate", func() error { return stageGenerate(version, out, env, excluded) }},
		{"gogenerate", func() error { return runIn(out, env, 5*time.Minute, "go", "generate", "./...") }},
		{"build", func() error { return runIn(out, env, 10*time.Minute, "go", "build", "./...") }},
		// The generated bindings pass pointers as uintptr/unsafe.Pointer by design; vet's unsafeptr check flags all of them.
		{"vet", func() error { return runIn(out, env, 10*time.Minute, "go", "vet", "-unsafeptr=false", "./...") }},
		{"test", func() error { return runIn(out, env, 10*time.Minute, "go", "test", "./...") }},
		{"golden", func() error { return stageGolden(version, out, statics) }},
	}

	for _, s := range stages {
		err := s.fn()

		if err != nil {
			t.Fatalf("stage %s: %v", s.name, err)
		}
		t.Logf("stage %s ok", s.name)
	}
}

// stageGenerate runs vk-gen twice, checks that the two outputs are identical, and leaves the output and a go.mod in out.
func stageGenerate(version, out string, env []string, excluded excludedTests) error {
	second := out + ".second"
	defer os.RemoveAll(second)

	for _, dir := range []string{out, second} {
		// The registry file name is written into every generated file header, so it is passed relative to a fixed directory.
		cmd := exec.Command(genPath, "-inFile", "vk-"+version+".xml", "-outDir", dir)
		cmd.Dir = registryDir
		cmd.Env = env
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("vk-gen failed: %v\n%s", err, tail(string(output)))
		}
		// vk-gen reports most problems through logrus.Error and carries on, so the exit status is not enough.
		if bad := errorLines(string(output)); len(bad) > 0 {
			return fmt.Errorf("vk-gen logged errors:\n%s", strings.Join(bad, "\n"))
		}
	}

	a, err := readTree(out, nil)
	if err != nil {
		return err
	}
	b, err := readTree(second, nil)
	if err != nil {
		return err
	}
	if diff := compareTrees(a, b); diff != "" {
		return fmt.Errorf("output differs between two runs of vk-gen on the same input:\n%s", diff)
	}

	// The golden copy leaves out static files, so removing excluded tests does not affect it.
	for _, name := range excluded.Files {
		if err := os.Remove(filepath.Join(out, name)); err != nil {
			return fmt.Errorf("excluded static test: %v", err)
		}
	}

	return writeModule(out)
}

// writeModule makes the output a standalone module, as in the go-vk repository. go.sum comes from vk-gen's own module so
// that x/sys needs no further network access once cached.
func writeModule(dir string) error {
	rootMod, err := os.ReadFile("../go.mod")
	if err != nil {
		return err
	}
	sysVersion := regexp.MustCompile(`golang.org/x/sys (v\S+)`).FindSubmatch(rootMod)
	if sysVersion == nil {
		return fmt.Errorf("could not find the golang.org/x/sys version in vk-gen's go.mod")
	}
	mod := fmt.Sprintf("module github.com/bbredesen/go-vk\n\ngo 1.23.0\n\nrequire golang.org/x/sys %s\n", sysVersion[1])
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o666); err != nil {
		return err
	}
	sum, err := os.ReadFile("../go.sum")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o666)
}

func stageGolden(version, out string, statics map[string]bool) error {
	skip := func(name string) bool { return testModuleFiles[name] || statics[name] }
	got, err := readTree(out, skip)
	if err != nil {
		return err
	}
	dir := filepath.Join(goldenDir, version)

	if *update {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return err
		}
		for name, data := range got {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o666); err != nil {
				return err
			}
		}
		return nil
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // smoke test only; there is no golden for this version
	}
	want, err := readTree(dir, nil)
	if err != nil {
		return err
	}
	if diff := compareTrees(want, got); diff != "" {
		return fmt.Errorf("output differs from %s (run `go test -count=1 ./matrix -update` and review `git diff` if the change is intended):\n%s", dir, diff)
	}
	return nil
}

// readTree reads the regular files of a flat directory.
func readTree(dir string, skip func(name string) bool) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || (skip != nil && skip(e.Name())) {
			continue
		}
		if files[e.Name()], err = os.ReadFile(filepath.Join(dir, e.Name())); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// compareTrees describes the differences between two sets of files, or returns "" if they are identical.
func compareTrees(want, got map[string][]byte) string {
	names := map[string]bool{}
	for n := range want {
		names[n] = true
	}
	for n := range got {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	var sb strings.Builder
	count := 0
	for _, n := range sorted {
		w, inWant := want[n]
		g, inGot := got[n]
		switch {
		case !inWant:
			fmt.Fprintf(&sb, "  + %s (not in the expected output)\n", n)
		case !inGot:
			fmt.Fprintf(&sb, "  - %s (missing from the output)\n", n)
		case !equivalent(n, w, g):
			fmt.Fprintf(&sb, "  ~ %s: %s\n", n, firstDifference(n, w, g))
		default:
			continue
		}
		if count++; count == 25 {
			sb.WriteString("  ...\n")
			break
		}
	}
	return sb.String()
}

// equivalent reports whether two versions of a file have the same content. Go files are compared as tokens, so that
// layout does not matter: gofmt's alignment of keys and comments changes between Go versions, and a golden recorded with
// one version would otherwise fail on another. Comments are tokens, so their text is still compared.
func equivalent(name string, want, got []byte) bool {
	if bytes.Equal(want, got) {
		return true
	}
	if !strings.HasSuffix(name, ".go") {
		return false
	}
	w, wok := goTokens(want)
	g, gok := goTokens(got)
	return wok && gok && slices.EqualFunc(w, g, func(a, b goToken) bool { return a.text == b.text })
}

// goTokens returns the tokens of Go source as strings, with the line each is on, and false if the source does not scan.
func goTokens(src []byte) (tokens []goToken, ok bool) {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	failed := false
	var sc scanner.Scanner
	sc.Init(file, src, func(token.Position, string) { failed = true }, scanner.ScanComments)
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if lit == "" {
			lit = tok.String()
		}
		tokens = append(tokens, goToken{line: file.Line(pos), text: lit})
	}
	return tokens, !failed
}

// goToken is compared by text only; line is for reporting.
type goToken struct {
	line int
	text string
}

func firstDifference(name string, want, got []byte) string {
	if strings.HasSuffix(name, ".go") {
		w, wok := goTokens(want)
		g, gok := goTokens(got)
		if wok && gok {
			for i := 0; i < len(w) || i < len(g); i++ {
				switch {
				case i >= len(w):
					return fmt.Sprintf("line %d: extra output starting at %q", g[i].line, g[i].text)
				case i >= len(g):
					return fmt.Sprintf("line %d: output ends early, expected %q", w[i].line, w[i].text)
				case w[i].text != g[i].text:
					return fmt.Sprintf("line %d (expected) / %d (got): expected %q, got %q", w[i].line, g[i].line, w[i].text, g[i].text)
				}
			}
		}
	}
	wl, gl := bytes.Split(want, []byte("\n")), bytes.Split(got, []byte("\n"))
	for i := 0; i < len(wl) || i < len(gl); i++ {
		switch {
		case i >= len(wl):
			return fmt.Sprintf("line %d: extra output %q", i+1, gl[i])
		case i >= len(gl):
			return fmt.Sprintf("line %d: output ends early, expected %q", i+1, wl[i])
		case !bytes.Equal(wl[i], gl[i]):
			return fmt.Sprintf("line %d: expected %q, got %q", i+1, wl[i], gl[i])
		}
	}
	return "contents differ"
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// errorLines returns the error-level (and worse) logrus lines in vk-gen's output, and any panic.
func errorLines(output string) []string {
	var bad []string
	for _, line := range strings.Split(ansi.ReplaceAllString(output, ""), "\n") {
		if strings.HasPrefix(line, "ERRO[") || strings.HasPrefix(line, "FATA[") || strings.HasPrefix(line, "PANI[") || strings.HasPrefix(line, "panic:") {
			bad = append(bad, "  "+line)
		}
	}
	return bad
}

// staticNames returns the names of the files that vk-gen copies from _static_include into every output.
func staticNames(t *testing.T) map[string]bool {
	entries, err := os.ReadDir(staticDir)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	return names
}

// toolEnv is the environment for vk-gen and the go commands. GOPATH/bin is added to PATH for stringer, and workspace
// mode is turned off so that no enclosing go.work applies to the generated module.
func toolEnv() []string {
	path := strings.Join(append(filepath.SplitList(os.Getenv("PATH")), gopathBins()...), string(os.PathListSeparator))
	// -mod=mod lets go add the x/sys requirement's go.sum entries if the copied go.sum lacks them.
	return append(os.Environ(), "PATH="+path, "GOWORK=off", "GOFLAGS=-mod=mod")
}

func runIn(dir string, env []string, timeout time.Duration, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, tail(buf.String()))
		}
		return nil
	case <-time.After(timeout):
		cmd.Process.Kill()
		return fmt.Errorf("%s %s: timed out after %v", name, strings.Join(args, " "), timeout)
	}
}

func requireTool(t *testing.T, name string) {
	t.Helper()
	// vk-gen and `go generate` look in GOPATH/bin too, so check there as well as PATH. LookPath adds the .exe extension
	// on Windows.
	path := strings.Join(append(filepath.SplitList(os.Getenv("PATH")), gopathBins()...), string(os.PathListSeparator))
	for _, dir := range filepath.SplitList(path) {
		if _, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return
		}
	}
	msg := fmt.Sprintf("%s not found in PATH or GOPATH/bin (go install golang.org/x/tools/cmd/goimports@latest / golang.org/x/tools/cmd/stringer@latest)", name)
	if os.Getenv("CI") != "" || os.Getenv("VK_MATRIX_REQUIRE_TOOLS") != "" {
		t.Fatal(msg)
	}
	t.Skip(msg)
}

func gopathBins() []string {
	out, _ := exec.Command("go", "env", "GOPATH").Output()
	var bins []string
	for _, p := range filepath.SplitList(strings.TrimSpace(string(out))) {
		bins = append(bins, filepath.Join(p, "bin"))
	}
	return bins
}

func tail(s string) string {
	lines := strings.Split(strings.TrimRight(ansi.ReplaceAllString(s, ""), "\n"), "\n")
	if len(lines) > 40 {
		lines = append([]string{"..."}, lines[len(lines)-40:]...)
	}
	return strings.Join(lines, "\n")
}

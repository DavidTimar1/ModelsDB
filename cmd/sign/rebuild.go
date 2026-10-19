package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// The release is only trusted when the maintainer's own build of the tag's
// commit has the same hashes as the binaries CI uploaded. That needs the commit's
// source, a build with build.sh's exact flags, and the commit's embedded key.

// gitPaths are the only places git is looked for; like gh, it is never found
// through PATH.
var gitPaths = []string{"/usr/bin/git", "/usr/local/bin/git", "/bin/git"}

// gitBinary finds git. It is a variable so a test can replace it.
var gitBinary = func() string {
	for _, p := range gitPaths {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// goBinary is the go command of the toolchain running this program, so the
// rebuild uses the same compiler the maintainer is running. It is taken from the
// root this program was built with and never searched for on PATH, the same rule
// gitBinary follows: a go command found through PATH is whatever the environment
// says it is. runtime.GOROOT is deprecated for programs that are copied to other
// machines, which this one never is; it runs from the maintainer's own checkout,
// and a root that is not absolute is refused by goRoot. A variable so a test can
// replace it.
var goBinary = func() string {
	//lint:ignore SA1019 this program is never copied to another machine; see above
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

// sourceURL is where the commit is fetched from. A variable so a test can point
// it at a local repository.
var sourceURL = func(repo string) string {
	return "https://github.com/" + repo + ".git"
}

// buildBinaries rebuilds the named release assets of a job from its source tree
// into its output folder. A variable so a test can replace the slow real build.
var buildBinaries = buildReleaseBinaries

// buildTargets maps each release asset to the GOOS it is built for. A name not
// listed here cannot be rebuilt, so it is not signed.
var buildTargets = map[string]string{
	"modelsdb":     "linux",
	"modelsdb.exe": "windows",
}

var (
	commitRe   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	versionRe  = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	pubKeyLine = regexp.MustCompile(`(?m)^const releasePubKeyB64 = "([A-Za-z0-9+/=]*)"$`)
)

// runIn runs name with args in dir, with the given extra environment, and
// returns its combined output.
func runIn(dir string, extraEnv []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// fetchSource puts exactly the commit sha of repo into dir, a new empty folder.
func fetchSource(repo, sha, dir string) error {
	git := gitBinary()
	if git == "" {
		return fmt.Errorf("git was not found at %s", strings.Join(gitPaths, ", "))
	}
	env := []string{"GIT_TERMINAL_PROMPT=0"}
	steps := [][]string{
		{"init", "-q"},
		{"-c", "core.hooksPath=/dev/null", "fetch", "-q", "--depth", "1", sourceURL(repo), sha},
		{"-c", "core.hooksPath=/dev/null", "-c", "advice.detachedHead=false", "checkout", "-q", "--detach", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if out, err := runIn(dir, env, git, args...); err != nil {
			return fmt.Errorf("git %s: %w\n%s", args[len(args)-1], err, cleanOutput(strings.TrimSpace(out)))
		}
	}
	got, err := runIn(dir, env, git, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(got) != sha {
		return fmt.Errorf("the fetched commit is %q, not %s", strings.TrimSpace(got), sha)
	}
	return nil
}

// plainFileIn returns the path of rel inside src after requiring every part of
// it, folders included, to be a real file or folder and not a link. The tagged
// commit is hostile input: a link could point at any file of this machine.
func plainFileIn(src string, rel ...string) (string, error) {
	cur := src
	for i, part := range rel {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return "", err
		}
		last := i == len(rel)-1
		if fi.Mode()&os.ModeSymlink != 0 || (last && !fi.Mode().IsRegular()) || (!last && !fi.IsDir()) {
			return "", fmt.Errorf("%s in the tag's commit is not a plain file", filepath.Join(rel...))
		}
	}
	return cur, nil
}

// embeddedKeyAt returns the base64 release public key the source tree in src
// embeds in internal/selfupdate/pubkey.go.
func embeddedKeyAt(src string) (string, error) {
	path, err := plainFileIn(src, "internal", "selfupdate", "pubkey.go")
	if err != nil {
		return "", fmt.Errorf("read the tag's pubkey.go: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read the tag's pubkey.go: %w", err)
	}
	m := pubKeyLine.FindAllStringSubmatch(string(raw), -1)
	if len(m) != 1 {
		return "", fmt.Errorf("the tag's pubkey.go does not hold exactly one releasePubKeyB64 constant")
	}
	return m[0][1], nil
}

// maxVersionFile is the most bytes of VERSION that are read.
const maxVersionFile = 64

// versionAt returns the release version the source tree in src declares. The
// file's content is never put in an error: only a value of the form X.Y.Z is
// ever returned or shown.
func versionAt(src string) (string, error) {
	path, err := plainFileIn(src, "VERSION")
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxVersionFile+1))
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(raw))
	if len(raw) > maxVersionFile || !versionRe.MatchString(v) {
		return "", errors.New("the tag's VERSION is not of the form X.Y.Z")
	}
	return v, nil
}

// buildJob is one rebuild: the fetched source, the release version, the assets
// to build, where they go, and the key file the build must not be able to read.
type buildJob struct {
	Src, Version, OutDir, KeyPath string
	Names                         []string
}

// buildEnv is the environment of one release build: no cgo, the target
// platform, no inherited Go flags or workspace, and the running toolchain only.
func buildEnv(goos string) []string {
	return append([]string{"CGO_ENABLED=0", "GOOS=" + goos, "GOARCH=amd64"}, cleanGoEnv...)
}

// cleanGoEnv fixes the Go settings the operator's environment could inject.
// GOTOOLCHAIN=local keeps the tag's go.mod from making go fetch another
// compiler: the build fails instead, and the message names the one in use.
var cleanGoEnv = []string{"GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local"}

// buildArgs are the go build arguments of one release build. They repeat the
// flags in build.sh: a different flag there gives different bytes here.
// -buildvcs=false keeps the bytes the same whether the source sits in a git
// checkout (CI, this rebuild) or is a plain copy.
func buildArgs(version, out string) []string {
	return []string{
		"build", "-trimpath", "-buildvcs=false", "-tags", "minified",
		"-ldflags", "-X 'modelsdb/internal/shared.Version=" + version + "'",
		"-o", out, "./cmd/modelsdb",
	}
}

// goRoot returns the Go root of the toolchain running this program. It must be
// an absolute folder holding bin/go: an empty runtime.GOROOT() would otherwise
// make the root "." and bind the current folder into the sandbox.
func goRoot() (string, error) {
	bin := goBinary()
	if !filepath.IsAbs(bin) {
		return "", fmt.Errorf("the Go root is not an absolute path (%q); run sign with go run, or build it without -trimpath", filepath.Dir(filepath.Dir(bin)))
	}
	if fi, err := os.Stat(bin); err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("the Go root %s holds no bin/go", filepath.Dir(filepath.Dir(bin)))
	}
	root := filepath.Dir(filepath.Dir(bin))
	if root == string(filepath.Separator) {
		return "", errors.New("the Go root is the whole file system; refusing to bind it into the sandbox")
	}
	return root, nil
}

// downloadModules fills a module cache for the source in src before the sandbox
// starts. `go mod download` fetches and checks modules against go.sum and runs
// none of their code. Its environment is built from scratch, and it runs under
// a time limit and a work-folder size limit.
func downloadModules(root, src, modcache, scratch string) error {
	env := append([]string{
		"PATH=" + filepath.Join(root, "bin"),
		"HOME=" + scratch,
		"GOPATH=" + filepath.Join(scratch, "gopath"),
		"GOMODCACHE=" + modcache,
		"GOCACHE=" + filepath.Join(scratch, "dlcache"),
		"GOFLAGS=-modcacherw", // lets the scratch folder be removed afterwards
	}, cleanGoEnvNoFlags()...)
	out, err := runLimited(downloadTimeout, filepath.Dir(src), func(ctx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(ctx, filepath.Join(root, "bin", "go"), "mod", "download")
		cmd.Dir = src
		cmd.Env = append([]string{}, env...)
		return cmd
	})
	if err != nil {
		return fmt.Errorf("download the tag's modules: %w\n%s", err, out)
	}
	return nil
}

// cleanGoEnvNoFlags is cleanGoEnv without GOFLAGS, for commands that set it.
func cleanGoEnvNoFlags() []string {
	var out []string
	for _, kv := range cleanGoEnv {
		if !strings.HasPrefix(kv, "GOFLAGS=") {
			out = append(out, kv)
		}
	}
	return out
}

// buildReleaseBinaries builds each named asset from the source as build.sh
// does. The commit's code runs only inside the sandbox.
func buildReleaseBinaries(job buildJob) error {
	if sandboxBinary() == "" {
		return sandboxMissing()
	}
	root, err := goRoot()
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp(filepath.Dir(job.Src), "buildenv-")
	if err != nil {
		return err
	}
	modcache := filepath.Join(scratch, "modcache")
	cache := filepath.Join(scratch, "gocache")
	for _, d := range []string{modcache, cache} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := downloadModules(root, job.Src, modcache, scratch); err != nil {
		return err
	}
	box, err := sandboxFor(job, root, cache, modcache)
	if err != nil {
		return err
	}
	goBin := filepath.Join(root, "bin", "go")
	run := func(extra []string, args ...string) (string, error) {
		b := box
		b.Env = append(append([]string{}, box.Env...), extra...)
		return runLimited(stepTimeout, filepath.Dir(job.Src), func(ctx context.Context) *exec.Cmd {
			return b.commandContext(ctx, job.Src, goBin, args...)
		})
	}
	if out, err := run(cleanGoEnv, "run", "-buildvcs=false", "./cmd/minifyassets"); err != nil {
		return fmt.Errorf("minify assets: %w\n%s", err, out)
	}
	for _, name := range job.Names {
		goos, ok := buildTargets[name]
		if !ok {
			return fmt.Errorf("%s cannot be rebuilt here, so it is not signed", name)
		}
		if out, err := run(buildEnv(goos), buildArgs(job.Version, filepath.Join(job.OutDir, name))...); err != nil {
			return fmt.Errorf("build %s: %w\n%s", name, err, out)
		}
	}
	return nil
}

// sandboxMissing is the refusal when bubblewrap is not installed.
func sandboxMissing() error {
	return fmt.Errorf("bubblewrap was not found at %s; publish builds the tagged commit's code only inside a sandbox, so it refuses without one (Linux only)", sandboxPath)
}

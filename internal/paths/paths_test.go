package paths

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestGlobalFlagArgs verifies that ParseGlobalFlags records the consumed global
// flags, normalized to "--flag=value", so a spawned child (background start,
// menu update) can be relaunched with the identical dir/port/host overrides.
func TestGlobalFlagArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		rest    []string
		want    []string
		unknown []string
	}{
		{"none", []string{"serve"}, []string{"serve"}, nil, nil},
		{"space form", []string{"--port", "9000", "start"}, []string{"start"}, []string{"--port=9000"}, nil},
		{"equals form", []string{"--port=9000"}, nil, []string{"--port=9000"}, nil},
		{"single dash normalized to long", []string{"-host", "100.1.2.3"}, nil, []string{"--host=100.1.2.3"}, nil},
		{"portable bool", []string{"--portable", "status"}, []string{"status"}, []string{"--portable"}, nil},
		{
			"mixed, flags around subcommand",
			[]string{"--config-dir", "/c", "start", "--host=100.1.2.3"},
			[]string{"start"},
			[]string{"--config-dir=/c", "--host=100.1.2.3"},
			nil,
		},
		// An unknown dash-prefixed token is surfaced as unknown (never forwarded to
		// a child, never mistaken for a command).
		{"unknown flag alone", []string{"--bogus"}, nil, nil, []string{"--bogus"}},
		{"unknown flag after subcommand", []string{"serve", "--nope"}, []string{"serve"}, nil, []string{"--nope"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rest, unknown := ParseGlobalFlags(c.args)
			if !slices.Equal(rest, c.rest) {
				t.Errorf("rest = %v, want %v", rest, c.rest)
			}
			if !slices.Equal(GlobalFlagArgs(), c.want) {
				t.Errorf("GlobalFlagArgs() = %v, want %v", GlobalFlagArgs(), c.want)
			}
			if !slices.Equal(unknown, c.unknown) {
				t.Errorf("unknown = %v, want %v", unknown, c.unknown)
			}
		})
	}
}

// TestHelpVersionFlags verifies the informational flags set the right package
// state, are not forwarded to a spawned child, and reset on the next parse.
func TestHelpVersionFlags(t *testing.T) {
	for _, f := range []string{"--help", "-help", "-h"} {
		ParseGlobalFlags([]string{f})
		if !HelpRequested() {
			t.Errorf("%s should set HelpRequested", f)
		}
		if VersionRequested() {
			t.Errorf("%s should not set VersionRequested", f)
		}
		if GlobalFlagArgs() != nil {
			t.Errorf("%s should not be forwarded to a child, got %v", f, GlobalFlagArgs())
		}
	}
	for _, f := range []string{"--version", "-version", "-v"} {
		ParseGlobalFlags([]string{f})
		if !VersionRequested() {
			t.Errorf("%s should set VersionRequested", f)
		}
		if HelpRequested() {
			t.Errorf("%s should not set HelpRequested", f)
		}
	}
	// A normal parse clears both flags.
	ParseGlobalFlags([]string{"serve"})
	if HelpRequested() || VersionRequested() {
		t.Error("help/version flags should reset on a flag-free parse")
	}
}

func TestStripJSONC(t *testing.T) {
	// A realistic JSONC config: line + block comments, a trailing comma, a string
	// value containing "//" (must NOT be treated as a comment), and an escaped
	// Windows path.
	jsonc := []byte(`{
  // the loopback port
  "port": 9123, // inline comment
  /* block comment
     spanning lines */
  "data_dir": "C:\\Users\\me\\data",
  "upstream": { "owner": "acme", "repo": "fork", "branch": "main" }, // note "https://x" stays
  "host": "https://not-a-comment // really",
  "allow_public": true,
}`)

	var c fileConfig
	if err := json.Unmarshal(stripJSONC(jsonc), &c); err != nil {
		t.Fatalf("JSONC config should parse after stripping: %v\n---stripped---\n%s", err, stripJSONC(jsonc))
	}
	if c.Port != 9123 {
		t.Errorf("port = %d, want 9123", c.Port)
	}
	if c.DataDir != `C:\Users\me\data` {
		t.Errorf("data_dir = %q (backslashes mangled)", c.DataDir)
	}
	if c.Upstream == nil || c.Upstream.Owner != "acme" || c.Upstream.Branch != "main" {
		t.Errorf("upstream = %+v", c.Upstream)
	}
	if c.Host != "https://not-a-comment // really" {
		t.Errorf("string value with // was altered: %q", c.Host)
	}
	if !c.AllowPublic {
		t.Error("allow_public should be true")
	}
}

func TestStripJSONCPreservesPlainJSON(t *testing.T) {
	// A comment-free strict-JSON config must pass through unchanged in meaning.
	plain := []byte(`{"port":8200,"portable":true}`)
	if got := strings.TrimSpace(string(stripJSONC(plain))); got != string(plain) {
		t.Errorf("plain JSON changed: %q -> %q", plain, got)
	}
}

func TestResolveConfigBase(t *testing.T) {
	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("no file -> canonical config.jsonc", func(t *testing.T) {
		dir := t.TempDir()
		if got := resolveConfigBase(dir); got != "config.jsonc" {
			t.Errorf("got %q, want config.jsonc", got)
		}
		if hasConfig(dir) {
			t.Error("hasConfig should be false for an empty dir")
		}
	})
	t.Run("legacy config.json is accepted", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "config.json")
		if got := resolveConfigBase(dir); got != "config.json" {
			t.Errorf("got %q, want config.json", got)
		}
		if !hasConfig(dir) {
			t.Error("hasConfig should be true when config.json exists")
		}
	})
	t.Run("config.jsonc is preferred over config.json", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "config.json")
		write(dir, "config.jsonc")
		if got := resolveConfigBase(dir); got != "config.jsonc" {
			t.Errorf("got %q, want config.jsonc (preferred)", got)
		}
	})
}

func TestApplyUpstream(t *testing.T) {
	origUp, origCfg := ActiveUpstream, AppConfig.Upstream
	t.Cleanup(func() { ActiveUpstream, AppConfig.Upstream = origUp, origCfg })

	clearEnv := func(t *testing.T) {
		t.Setenv("MODELSDB_UPSTREAM_OWNER", "")
		t.Setenv("MODELSDB_UPSTREAM_REPO", "")
		t.Setenv("MODELSDB_UPSTREAM_BRANCH", "")
	}

	t.Run("default when nothing set", func(t *testing.T) {
		clearEnv(t)
		AppConfig.Upstream = nil
		applyUpstream()
		if ActiveUpstream != defaultUpstream() {
			t.Errorf("upstream = %+v, want default", ActiveUpstream)
		}
	})
	t.Run("config.json upstream applies (branch defaults to main)", func(t *testing.T) {
		clearEnv(t)
		AppConfig.Upstream = &Upstream{Owner: "acme", Repo: "fork"}
		applyUpstream()
		if ActiveUpstream.Owner != "acme" || ActiveUpstream.Repo != "fork" || ActiveUpstream.Branch != "main" {
			t.Errorf("config upstream not applied: %+v", ActiveUpstream)
		}
	})
	t.Run("env overrides config per field", func(t *testing.T) {
		clearEnv(t)
		AppConfig.Upstream = &Upstream{Owner: "acme", Repo: "fork", Branch: "main"}
		t.Setenv("MODELSDB_UPSTREAM_OWNER", "bob")
		t.Setenv("MODELSDB_UPSTREAM_BRANCH", "dev")
		applyUpstream()
		if ActiveUpstream.Owner != "bob" || ActiveUpstream.Repo != "fork" || ActiveUpstream.Branch != "dev" {
			t.Errorf("env override wrong: %+v", ActiveUpstream)
		}
	})
}

func TestResolvePathsDevOverride(t *testing.T) {
	// This exercises the `go run` (development) branch of ResolvePaths, which only
	// runs when the executable looks like a temp/go-build binary - exactly what a
	// `go test` binary is. Skip rather than assert if that is somehow not the case.
	if p, err := os.Executable(); err == nil {
		ExePath = p
	}
	if !isDevRun() {
		t.Skip("test binary is not detected as a dev run; ResolvePaths dev branch not exercised")
	}

	// ResolvePaths mutates package globals and the current directory; snapshot and
	// restore everything so the test leaves no trace.
	origCwd, _ := os.Getwd()
	origCfgDir, origDataDir, origCacheDir, origDirName, origBase := ConfigDir, DataDir, CacheDir, DirName, configBase
	origAppCfg, origActiveUp, origPort := AppConfig, ActiveUpstream, Port
	origFlagCfg, origFlagData, origFlagCache := flagConfigDir, flagDataDir, flagCacheDir
	t.Cleanup(func() {
		ConfigDir, DataDir, CacheDir, DirName, configBase = origCfgDir, origDataDir, origCacheDir, origDirName, origBase
		AppConfig, ActiveUpstream, Port = origAppCfg, origActiveUp, origPort
		flagConfigDir, flagDataDir, flagCacheDir = origFlagCfg, origFlagData, origFlagCache
	})
	flagConfigDir, flagDataDir, flagCacheDir = "", "", ""

	cwd := t.TempDir()
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	// Restore the working directory BEFORE t.TempDir's removal runs (cleanups are
	// LIFO, so this must be registered after t.TempDir): on Windows a directory
	// that is a process's cwd cannot be deleted.
	t.Cleanup(func() { os.Chdir(origCwd) })
	// On macOS t.TempDir() lives under /var -> /private/var symlink; resolve so the
	// expected paths match what ResolvePaths derives from os.Getwd.
	cwd, _ = os.Getwd()

	t.Run("no override defaults to ./data", func(t *testing.T) {
		t.Setenv("MODELSDB_CONFIG_DIR", "")
		t.Setenv("MODELSDB_DATA_DIR", "")
		t.Setenv("MODELSDB_CACHE_DIR", "")
		flagConfigDir, flagDataDir, flagCacheDir = "", "", ""
		if err := ResolvePaths(); err != nil {
			t.Fatalf("ResolvePaths: %v", err)
		}
		want := filepath.Join(cwd, "data")
		if ConfigDir != want || DataDir != want || CacheDir != want {
			t.Errorf("defaults wrong:\n config=%q\n data=%q\n cache=%q\n want all %q", ConfigDir, DataDir, CacheDir, want)
		}
	})

	t.Run("config dir's config.jsonc supplies data_dir and cache_dir", func(t *testing.T) {
		t.Setenv("MODELSDB_DATA_DIR", "")
		t.Setenv("MODELSDB_CACHE_DIR", "")
		flagConfigDir, flagDataDir, flagCacheDir = "", "", ""
		// Mirror a shipped build's dist/config.jsonc: data beside the build, cache in
		// a subfolder of it.
		cfgDir := filepath.Join(cwd, "dist")
		if err := os.MkdirAll(cfgDir, 0755); err != nil {
			t.Fatal(err)
		}
		cfg := `{ "data_dir": "../data", "cache_dir": "../data/.cache", "port": 8122 }`
		if err := os.WriteFile(filepath.Join(cfgDir, "config.jsonc"), []byte(cfg), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("MODELSDB_CONFIG_DIR", cfgDir)

		if err := ResolvePaths(); err != nil {
			t.Fatalf("ResolvePaths: %v", err)
		}
		if ConfigDir != cfgDir {
			t.Errorf("ConfigDir = %q, want %q", ConfigDir, cfgDir)
		}
		if want := filepath.Join(cwd, "data"); DataDir != want {
			t.Errorf("DataDir = %q, want %q (from config data_dir)", DataDir, want)
		}
		if want := filepath.Join(cwd, "data", ".cache"); CacheDir != want {
			t.Errorf("CacheDir = %q, want %q (from config cache_dir)", CacheDir, want)
		}
		if want := filepath.Join(cwd, "data", ".cache", "modelsdb.pid"); PidFile != want {
			t.Errorf("PidFile = %q, want %q", PidFile, want)
		}
	})

	t.Run("explicit data-dir env wins over the config file", func(t *testing.T) {
		cfgDir := filepath.Join(cwd, "dist")
		override := filepath.Join(cwd, "elsewhere")
		t.Setenv("MODELSDB_CONFIG_DIR", cfgDir)
		t.Setenv("MODELSDB_DATA_DIR", override)
		t.Setenv("MODELSDB_CACHE_DIR", "")
		flagConfigDir, flagDataDir, flagCacheDir = "", "", ""
		if err := ResolvePaths(); err != nil {
			t.Fatalf("ResolvePaths: %v", err)
		}
		if DataDir != override {
			t.Errorf("DataDir = %q, want explicit override %q", DataDir, override)
		}
	})
}

// TestIsTailscaleIP verifies the Tailscale/CGNAT range check (100.64.0.0/10):
// the endpoints of the block are in, addresses just outside it are not, and
// ordinary private/loopback addresses are not.
func TestIsTailscaleIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.255", true},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
		{"10.0.0.1", false},
		{"127.0.0.1", false},
	}
	for _, c := range cases {
		t.Run(c.ip, func(t *testing.T) {
			ip := net.ParseIP(c.ip)
			if ip == nil {
				t.Fatalf("bad test IP %q", c.ip)
			}
			if got := isTailscaleIP(ip); got != c.want {
				t.Errorf("isTailscaleIP(%s) = %v, want %v", c.ip, got, c.want)
			}
		})
	}
}

// TestBindExposure verifies the exposure classifier across IP literals: loopback
// aliases, wildcard binds, private/link-local ranges, the Tailscale range, and a
// routable public address (which the app layer refuses without --allow-public).
func TestBindExposure(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"", "loopback"},
		{"127.0.0.1", "loopback"},
		{"localhost", "loopback"},
		{"::1", "loopback"},
		{"0.0.0.0", "wildcard"},
		{"::", "wildcard"},
		{"100.85.27.66", "tailscale"},
		{"192.168.1.5", "private"},
		{"10.0.0.1", "private"},
		{"172.16.0.1", "private"},
		{"169.254.1.1", "private"},
		{"fc00::1", "private"},
		{"fe80::1", "private"},
		{"8.8.8.8", "public"},
		{"1.1.1.1", "public"},
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			if got := BindExposure(c.host); got != c.want {
				t.Errorf("BindExposure(%q) = %q, want %q", c.host, got, c.want)
			}
		})
	}
}

// TestMigrateLegacyDir verifies the explicit-path rename logic: a legacy-cased
// directory with a file migrates when the new path is absent, is a no-op when
// the new path already exists, and is a no-op when no segment matches the leaf.
func TestMigrateLegacyDir(t *testing.T) {
	sep := string(os.PathSeparator)

	t.Run("migrates when new path absent", func(t *testing.T) {
		root := t.TempDir()
		oldPath := filepath.Join(root, legacyAppName)
		newPath := filepath.Join(root, appName)
		if err := os.MkdirAll(oldPath, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(oldPath, "modelsdb.db"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}

		migrateLegacyDir(newPath)

		if _, err := os.Stat(filepath.Join(newPath, "modelsdb.db")); err != nil {
			t.Errorf("file not migrated to new path: %v", err)
		}
		if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
			t.Errorf("old path should be gone, stat err = %v", err)
		}
	})

	t.Run("no-op when new path already exists", func(t *testing.T) {
		root := t.TempDir()
		oldPath := filepath.Join(root, legacyAppName)
		newPath := filepath.Join(root, appName)
		if err := os.MkdirAll(filepath.Join(oldPath, "old"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(newPath, "new"), 0755); err != nil {
			t.Fatal(err)
		}

		migrateLegacyDir(newPath)

		// Both survive untouched: no rename happened.
		if _, err := os.Stat(filepath.Join(oldPath, "old")); err != nil {
			t.Errorf("old path should be untouched: %v", err)
		}
		if _, err := os.Stat(filepath.Join(newPath, "new")); err != nil {
			t.Errorf("new path should be untouched: %v", err)
		}
	})

	t.Run("no-op when no segment matches the leaf", func(t *testing.T) {
		root := t.TempDir()
		newPath := filepath.Join(root, "somewhere", "else")
		if err := os.MkdirAll(newPath, 0755); err != nil {
			t.Fatal(err)
		}
		// A same-named legacy twin cannot exist because no segment equals appName,
		// so oldPath == newPath and the call returns without touching anything.
		migrateLegacyDir(newPath)
		after, err := os.Stat(newPath)
		if err != nil || !after.IsDir() {
			t.Errorf("new path should be untouched: %v", err)
		}
		// Sanity: deriving oldPath from a leaf-free path yields the same string.
		segs := strings.Split(newPath, sep)
		if strings.Join(segs, sep) != newPath {
			t.Fatalf("path round-trip mismatch")
		}
	})
}

// TestResolveAllowPublic verifies the first-hit-wins cascade: config value,
// env override (truthy words only), and the flag forcing it on.
func TestResolveAllowPublic(t *testing.T) {
	origCfg, origFlag, origVal := AppConfig.AllowPublic, flagAllowPublic, AllowPublic
	t.Cleanup(func() { AppConfig.AllowPublic, flagAllowPublic, AllowPublic = origCfg, origFlag, origVal })

	set := func(cfg, flag bool) { AppConfig.AllowPublic, flagAllowPublic = cfg, flag }

	t.Run("default false", func(t *testing.T) {
		t.Setenv("MODELSDB_ALLOW_PUBLIC", "")
		set(false, false)
		resolveAllowPublic()
		if AllowPublic {
			t.Error("AllowPublic should default to false")
		}
	})
	t.Run("config true is used", func(t *testing.T) {
		t.Setenv("MODELSDB_ALLOW_PUBLIC", "")
		set(true, false)
		resolveAllowPublic()
		if !AllowPublic {
			t.Error("config allow_public=true should be used")
		}
	})
	t.Run("env truthy overrides config false", func(t *testing.T) {
		t.Setenv("MODELSDB_ALLOW_PUBLIC", "yes")
		set(false, false)
		resolveAllowPublic()
		if !AllowPublic {
			t.Error("MODELSDB_ALLOW_PUBLIC=yes should enable")
		}
	})
	t.Run("env falsey overrides config true", func(t *testing.T) {
		t.Setenv("MODELSDB_ALLOW_PUBLIC", "0")
		set(true, false)
		resolveAllowPublic()
		if AllowPublic {
			t.Error("MODELSDB_ALLOW_PUBLIC=0 should disable")
		}
	})
	t.Run("flag forces on", func(t *testing.T) {
		t.Setenv("MODELSDB_ALLOW_PUBLIC", "")
		set(false, true)
		resolveAllowPublic()
		if !AllowPublic {
			t.Error("--allow-public should force enable")
		}
	})
}

func TestResolvePort(t *testing.T) {
	origCfg, origFlag, origPort := AppConfig.Port, flagPort, Port
	t.Cleanup(func() { AppConfig.Port, flagPort, Port = origCfg, origFlag, origPort })

	set := func(cfg int, flag string) { AppConfig.Port, flagPort = cfg, flag }

	t.Run("default when nothing set", func(t *testing.T) {
		t.Setenv("MODELSDB_PORT", "")
		set(0, "")
		resolvePort()
		if Port != DefaultPort {
			t.Errorf("Port = %d, want default %d", Port, DefaultPort)
		}
	})
	t.Run("config.json port is used", func(t *testing.T) {
		t.Setenv("MODELSDB_PORT", "")
		set(9000, "")
		resolvePort()
		if Port != 9000 {
			t.Errorf("Port = %d, want 9000", Port)
		}
	})
	t.Run("env overrides config", func(t *testing.T) {
		t.Setenv("MODELSDB_PORT", "9100")
		set(9000, "")
		resolvePort()
		if Port != 9100 {
			t.Errorf("Port = %d, want 9100", Port)
		}
	})
	t.Run("flag overrides env and config", func(t *testing.T) {
		t.Setenv("MODELSDB_PORT", "9100")
		set(9000, "9200")
		resolvePort()
		if Port != 9200 {
			t.Errorf("Port = %d, want 9200", Port)
		}
	})
	t.Run("invalid config falls back to default", func(t *testing.T) {
		t.Setenv("MODELSDB_PORT", "")
		set(70000, "")
		resolvePort()
		if Port != DefaultPort {
			t.Errorf("out-of-range config port should fall back to default, got %d", Port)
		}
	})
	t.Run("invalid flag falls back to config", func(t *testing.T) {
		t.Setenv("MODELSDB_PORT", "")
		set(9000, "abc")
		resolvePort()
		if Port != 9000 {
			t.Errorf("unparseable flag should fall back to config, got %d", Port)
		}
	})
}

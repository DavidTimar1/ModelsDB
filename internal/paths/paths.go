// Package paths - on-disk location resolution.
//
// A downloaded binary must store its data somewhere sensible per OS, and a power
// user must be able to relocate config/data/cache independently. ResolvePaths
// implements that cascade (first hit wins, per directory):
//
//  1. CLI flag        --config-dir / --data-dir / --cache-dir
//  2. Env var         MODELSDB_CONFIG_DIR / MODELSDB_DATA_DIR / MODELSDB_CACHE_DIR
//  3. config.jsonc (or config.json) next to the exe   (portable mode)
//  4. config.jsonc (or config.json) in the OS config dir
//  5. OS-standard default           (UserConfigDir / platform data dir / UserCacheDir)
//
// On a first run with no config file, a config.jsonc is written into the resolved config
// dir recording the locations, so the app finds them next time. Development runs
// (`go run`) default config/data/cache to ./data and never write a config file, but
// an EXPLICIT override still wins: --config-dir/--data-dir/--cache-dir or the
// MODELSDB_*_DIR env vars set a dir directly, and a config dir's own config.jsonc
// supplies data_dir/cache_dir/port. This lets a wrapper point the dev server at the
// same data a shipped build uses.
package paths

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Upstream identifies the GitHub repo a build pulls its curated catalog and
// update notifications from. Forks set it in config.json to track their own repo
// instead of the canonical one.
type Upstream struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

func defaultUpstream() Upstream {
	return Upstream{Owner: "DavidTimar1", Repo: "ModelsDB", Branch: "main"}
}

// FileExists reports whether path exists and is a regular file (not a directory).
// It lives here in the lowest-level package so the path cascade and higher layers
// (db migrate, seed) can share one definition without an import cycle.
func FileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// configNames are the accepted config file base names, in preference order: the
// canonical config.jsonc first, then a legacy config.json of the same shape.
var configNames = []string{"config.jsonc", "config.json"}

// configBase is the config file base name actually in use, resolved at startup to
// the existing file's name (config.jsonc preferred), or the canonical name when
// none exists yet. finalizePaths derives ConfigFile from it.
var configBase = configFileName

// hasConfig reports whether dir contains a config file under any accepted name.
func hasConfig(dir string) bool {
	for _, n := range configNames {
		if FileExists(filepath.Join(dir, n)) {
			return true
		}
	}
	return false
}

// resolveConfigBase returns the base name of the config file to use in dir: an
// existing file (config.jsonc preferred, then config.json), or the canonical
// configFileName when none exists, so a first-run write creates config.jsonc.
func resolveConfigBase(dir string) string {
	for _, n := range configNames {
		if FileExists(filepath.Join(dir, n)) {
			return n
		}
	}
	return configFileName
}

// fileConfig is the on-disk config.json: a small pointer recording where the
// data/cache dirs live (the config dir is wherever this file is found) plus
// per-fork settings. Empty path fields fall back to the OS-standard default for
// that directory; relative paths resolve from the config.json's own directory
// (so "." means "the folder this config.json sits in").
type fileConfig struct {
	Portable    bool      `json:"portable,omitempty"`     // true => data + cache also = the config dir
	DataDir     string    `json:"data_dir,omitempty"`     // relative to the config dir; "." = the config dir
	CacheDir    string    `json:"cache_dir,omitempty"`    // relative to the config dir; "." = the config dir
	Port        int       `json:"port,omitempty"`         // bind port (1-65535); 0/absent => DefaultPort
	Host        string    `json:"host,omitempty"`         // extra bind address (a Tailscale 100.x IP, a private IP, or "tailscale" to auto-detect this machine's Tailscale IP); ""/absent => DefaultHost (loopback only)
	AllowPublic bool      `json:"allow_public,omitempty"` // permit binding 0.0.0.0 or any public address (off by default; a public bind is refused without it)
	Upstream    *Upstream `json:"upstream,omitempty"`
}

// CLI directory overrides (populated by ParseGlobalFlags); highest precedence.
var (
	flagConfigDir   string
	flagDataDir     string
	flagCacheDir    string
	flagPort        string
	flagHost        string
	flagPortable    bool
	flagAllowPublic bool
	flagHelp        bool // --help / -h: print usage and exit
	flagVersion     bool // --version / -v: print the version and exit
)

// HelpRequested reports whether a help flag (--help, -help, or -h) was passed
// to the most recent ParseGlobalFlags call.
func HelpRequested() bool { return flagHelp }

// VersionRequested reports whether a version flag (--version, -version, or -v)
// was passed to the most recent ParseGlobalFlags call.
func VersionRequested() bool { return flagVersion }

// globalFlagArgs holds the global flags consumed by ParseGlobalFlags, normalized
// to the "--flag=value" form, so a spawned child process (background start,
// menu-driven update) can be launched with the exact same directory/port/host
// overrides. Exposed via GlobalFlagArgs.
var globalFlagArgs []string

// GlobalFlagArgs returns the global flags this process was launched with,
// normalized to "--flag=value", for forwarding to a spawned child.
func GlobalFlagArgs() []string { return globalFlagArgs }

// ParseGlobalFlags extracts the global directory overrides from args (accepted
// in any position, before or after a subcommand). It accepts "--flag value",
// "--flag=value", and the single-dash forms. It returns rest - the non-flag
// tokens (the subcommand plus its operands) - and unknown - any dash-prefixed
// token that is not a recognized global flag, so the caller can reject it
// instead of mistaking it for a command. The help/version flags set package
// state read via HelpRequested/VersionRequested and are not forwarded to a
// spawned child.
func ParseGlobalFlags(args []string) (rest, unknown []string) {
	globalFlagArgs = nil
	flagHelp, flagVersion = false, false
	for i := 0; i < len(args); i++ {
		name, val, hasEq := strings.Cut(args[i], "=")
		take := func() string {
			if hasEq {
				return val
			}
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		// canonical is the long "--flag" form recorded for forwarding to children.
		keep := func(canonical, value string) {
			globalFlagArgs = append(globalFlagArgs, canonical+"="+value)
		}
		switch name {
		case "--config-dir", "-config-dir":
			flagConfigDir = take()
			keep("--config-dir", flagConfigDir)
		case "--data-dir", "-data-dir":
			flagDataDir = take()
			keep("--data-dir", flagDataDir)
		case "--cache-dir", "-cache-dir":
			flagCacheDir = take()
			keep("--cache-dir", flagCacheDir)
		case "--port", "-port":
			flagPort = take()
			keep("--port", flagPort)
		case "--host", "-host":
			flagHost = take()
			keep("--host", flagHost)
		case "--portable", "-portable":
			flagPortable = true
			globalFlagArgs = append(globalFlagArgs, "--portable")
		case "--allow-public", "-allow-public":
			flagAllowPublic = true
			globalFlagArgs = append(globalFlagArgs, "--allow-public")
		case "--help", "-help", "-h":
			flagHelp = true
		case "--version", "-version", "-v":
			flagVersion = true
		default:
			// A dash-prefixed token that matched no flag is an unknown option;
			// surface it so the caller rejects it rather than treating it as a
			// command. Everything else is a subcommand or an operand.
			if strings.HasPrefix(args[i], "-") {
				unknown = append(unknown, args[i])
			} else {
				rest = append(rest, args[i])
			}
		}
	}
	return rest, unknown
}

// isDevRun reports whether this is a `go run` build (its temp exe lives under a
// go-build cache / the OS temp dir), in which case paths stay under ./data.
func isDevRun() bool {
	p := strings.ToLower(ExePath)
	return strings.Contains(p, "go-build") || strings.Contains(p, string(filepath.Separator)+"temp"+string(filepath.Separator)) || strings.Contains(p, "/temp/")
}

// osDefaultDirs returns the OS-standard config/data/cache directories for the
// app. On Windows data and cache both live under %LOCALAPPDATA% (UserCacheDir),
// so each gets its own leaf to avoid colliding.
func osDefaultDirs() (config, data, cache string) {
	if c, err := os.UserConfigDir(); err == nil {
		config = filepath.Join(c, appName)
	}
	if c, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(c, appName)
	}
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LocalAppData")
		if local == "" {
			local, _ = os.UserCacheDir()
		}
		data = filepath.Join(local, appName, "data")
		cache = filepath.Join(local, appName, "cache")
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			data = filepath.Join(home, "Library", "Application Support", appName)
		}
	default: // linux & other unix - XDG data dir
		if x := os.Getenv("XDG_DATA_HOME"); x != "" {
			data = filepath.Join(x, appName)
		} else if home, err := os.UserHomeDir(); err == nil {
			data = filepath.Join(home, ".local", "share", appName)
		}
	}
	return
}

// migrateLegacyDirs renames each OS-default directory left by an older build
// (capitalized leaf, legacyAppName) forward to the current lowercase leaf, so an
// existing install's config/data/cache is not orphaned. It is a no-op for any
// dir that has already migrated or was never capitalized.
func migrateLegacyDirs(dirs ...string) {
	for _, d := range dirs {
		migrateLegacyDir(d)
	}
}

// migrateLegacyDir renames the legacy-cased twin of newPath to newPath. It
// derives the old path by replacing any path SEGMENT equal to appName
// ("modelsdb") with legacyAppName ("ModelsDB") - handling both the Linux/macOS
// case (the leaf is the app segment) and the Windows case (a middle segment like
// ...\modelsdb\data). It acts ONLY when newPath does not exist and the legacy
// path is an existing directory, so it never overwrites; on a case-insensitive
// filesystem oldPath and newPath are the same directory, os.Stat(newPath)
// succeeds, and nothing is renamed. A rename error is logged and startup
// continues.
func migrateLegacyDir(newPath string) {
	if newPath == "" {
		return
	}
	sep := string(os.PathSeparator)
	segs := strings.Split(newPath, sep)
	for i, s := range segs {
		if s == appName {
			segs[i] = legacyAppName
		}
	}
	oldPath := strings.Join(segs, sep)
	if oldPath == newPath {
		return
	}
	if _, err := os.Stat(newPath); err == nil {
		return // already migrated, or a case-insensitive FS where old == new
	}
	st, err := os.Stat(oldPath)
	if err != nil || !st.IsDir() {
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		log.Printf("Warning: could not migrate data directory %s -> %s: %v", oldPath, newPath, err)
		return
	}
	log.Printf("Migrated data directory %s -> %s", oldPath, newPath)
}

func resolveRel(base, p string) string {
	switch {
	case p == "":
		return ""
	case p == ".":
		return base
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	default:
		return filepath.Join(base, p)
	}
}

func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func loadConfigFile(path string) (fileConfig, bool) {
	var c fileConfig
	b, err := os.ReadFile(path)
	if err != nil {
		return c, false
	}
	// config.json is JSONC: it may carry // and /* */ comments and trailing commas
	// (so it can be self-documented). Strip those to strict JSON before decoding.
	if err := json.Unmarshal(stripJSONC(b), &c); err != nil {
		log.Printf("Warning: ignoring malformed %s: %v", path, err)
		return fileConfig{}, false
	}
	return c, true
}

// stripJSONC returns src with JSONC niceties removed so encoding/json (strict
// JSON) can parse it: "//" line comments, "/* */" block comments, and trailing
// commas before } or ]. Comment markers and commas INSIDE JSON strings are
// preserved, so string values like URLs ("http://...") or Windows paths are safe.
func stripJSONC(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inStr := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inStr {
			out = append(out, c)
			switch c {
			case '\\': // keep an escaped char verbatim (e.g. \" or \\)
				if i+1 < len(src) {
					i++
					out = append(out, src[i])
				}
			case '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n') // keep line structure for error messages
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i++ // skip the '*'; the loop's i++ skips the closing '/'
		default:
			out = append(out, c)
		}
	}
	return dropTrailingCommas(out)
}

// dropTrailingCommas removes a comma that is immediately followed (ignoring
// whitespace) by } or ], outside of strings.
func dropTrailingCommas(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inStr := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inStr {
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				i++
				out = append(out, src[i])
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				continue // skip the trailing comma
			}
		}
		out = append(out, c)
	}
	return out
}

// SaveConfigFile persists AppConfig to ConfigFile (creating the config dir). It
// writes the config as JSONC: a comment header documenting every option, then the
// JSON body. The app only writes this on first run, so comments a user adds
// afterward are left untouched.
func SaveConfigFile() error {
	if ConfigFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ConfigFile), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(AppConfig, "", "  ")
	if err != nil {
		return err
	}
	out := append([]byte(configHeader), b...)
	out = append(out, '\n')
	return os.WriteFile(ConfigFile, out, 0644)
}

// ResolvePaths runs the cascade described in the package doc, loads the config file,
// derives every file path, creates the directories, and writes a config.jsonc on
// first run so the locations are remembered.
func ResolvePaths() error {
	var err error
	if ExePath, err = os.Executable(); err != nil {
		return err
	}
	exeDir := filepath.Dir(ExePath)

	if isDevRun() {
		// Development (`go run`): default everything to ./data, which is already
		// git-ignored except curated.json, so the maintainer's curated.json
		// round-trips and the working tree stays clean. An EXPLICIT override still
		// wins, so a wrapper can point the dev server at a different config/data/
		// cache: --config-dir/--data-dir/--cache-dir or MODELSDB_*_DIR set the dir
		// directly, and a config dir's own config.jsonc supplies data_dir/cache_dir/
		// port/upstream. No config file is written in dev.
		cwd, _ := os.Getwd()
		base := filepath.Join(cwd, "data")
		DirName = cwd

		ConfigDir = FirstNonEmpty(flagConfigDir, os.Getenv("MODELSDB_CONFIG_DIR"), base)
		configBase = resolveConfigBase(ConfigDir)
		AppConfig, _ = loadConfigFile(filepath.Join(ConfigDir, configBase))
		applyUpstream()
		resolvePort()
		resolveHost()
		resolveAllowPublic()

		DataDir = FirstNonEmpty(flagDataDir, os.Getenv("MODELSDB_DATA_DIR"), resolveRel(ConfigDir, AppConfig.DataDir))
		if DataDir == "" {
			DataDir = base
		}
		CacheDir = FirstNonEmpty(flagCacheDir, os.Getenv("MODELSDB_CACHE_DIR"), resolveRel(ConfigDir, AppConfig.CacheDir))
		if CacheDir == "" {
			CacheDir = base
		}

		finalizePaths()
		return makeDirs()
	}

	DirName = exeDir
	osCfg, osData, osCache := osDefaultDirs()

	// Rename any directory an older build created under the capitalized leaf to
	// the current lowercase leaf, so an existing install's data is not orphaned.
	migrateLegacyDirs(osCfg, osData, osCache)

	// 1. CONFIG dir. A config.jsonc (or legacy config.json) next to the exe puts the
	// app in portable mode.
	portable := flagPortable
	ConfigDir = FirstNonEmpty(flagConfigDir, os.Getenv("MODELSDB_CONFIG_DIR"))
	if ConfigDir == "" {
		if hasConfig(exeDir) {
			ConfigDir, portable = exeDir, true
		} else {
			ConfigDir = osCfg
		}
	}

	// 2. Load the config file (config.jsonc preferred, legacy config.json) from the
	// resolved config dir.
	configBase = resolveConfigBase(ConfigDir)
	var found bool
	AppConfig, found = loadConfigFile(filepath.Join(ConfigDir, configBase))
	if AppConfig.Portable {
		portable = true
	}
	applyUpstream()
	resolvePort()
	resolveHost()
	resolveAllowPublic()

	// 3. DATA dir, 4. CACHE dir.
	DataDir = FirstNonEmpty(flagDataDir, os.Getenv("MODELSDB_DATA_DIR"), resolveRel(ConfigDir, AppConfig.DataDir))
	if DataDir == "" {
		DataDir = ifPortable(portable, ConfigDir, osData)
	}
	CacheDir = FirstNonEmpty(flagCacheDir, os.Getenv("MODELSDB_CACHE_DIR"), resolveRel(ConfigDir, AppConfig.CacheDir))
	if CacheDir == "" {
		CacheDir = ifPortable(portable, ConfigDir, osCache)
	}

	finalizePaths()
	if err := makeDirs(); err != nil {
		return err
	}

	// 5. First run with no config file: write a config.jsonc so the locations are remembered.
	if !found {
		if err := SaveConfigFile(); err != nil {
			log.Printf("Warning: could not write %s: %v", ConfigFile, err)
		}
	}
	return nil
}

func ifPortable(portable bool, yes, no string) string {
	if portable {
		return yes
	}
	return no
}

// applyUpstream resolves the upstream repo (catalog pull + update check) with the
// same first-hit-wins spirit as the other settings: start from the default, let a
// config.json "upstream" block replace it, then let MODELSDB_UPSTREAM_OWNER /
// _REPO / _BRANCH env vars override individual fields for a single run (no file
// edit) - parity with the MODELSDB_*_DIR / MODELSDB_PORT vars.
func applyUpstream() {
	ActiveUpstream = defaultUpstream()
	if AppConfig.Upstream != nil && AppConfig.Upstream.Owner != "" && AppConfig.Upstream.Repo != "" {
		ActiveUpstream = *AppConfig.Upstream
	}
	if v := strings.TrimSpace(os.Getenv("MODELSDB_UPSTREAM_OWNER")); v != "" {
		ActiveUpstream.Owner = v
	}
	if v := strings.TrimSpace(os.Getenv("MODELSDB_UPSTREAM_REPO")); v != "" {
		ActiveUpstream.Repo = v
	}
	if v := strings.TrimSpace(os.Getenv("MODELSDB_UPSTREAM_BRANCH")); v != "" {
		ActiveUpstream.Branch = v
	}
	if ActiveUpstream.Branch == "" {
		ActiveUpstream.Branch = "main"
	}
}

// resolvePort resolves the loopback port with the same first-hit-wins cascade as
// the directories: --port flag > MODELSDB_PORT env > config.json "port" > DefaultPort.
// An out-of-range or unparseable value is ignored with a warning, so a typo can
// never leave the server unable to bind.
func resolvePort() {
	Port = DefaultPort
	apply := func(p int, src string) {
		if p >= 1 && p <= 65535 {
			Port = p
		} else if p != 0 {
			log.Printf("Warning: ignoring invalid port %d from %s (must be 1-65535)", p, src)
		}
	}
	apply(AppConfig.Port, "config.json")
	if env := strings.TrimSpace(os.Getenv("MODELSDB_PORT")); env != "" {
		if p, err := strconv.Atoi(env); err == nil {
			apply(p, "MODELSDB_PORT")
		} else {
			log.Printf("Warning: ignoring invalid MODELSDB_PORT %q (must be 1-65535)", env)
		}
	}
	if f := strings.TrimSpace(flagPort); f != "" {
		if p, err := strconv.Atoi(f); err == nil {
			apply(p, "--port")
		} else {
			log.Printf("Warning: ignoring invalid --port %q (must be 1-65535)", f)
		}
	}
}

// resolveHost resolves the extra bind address with the same first-hit-wins
// cascade as the port: --host flag > MODELSDB_HOST env > config.json "host" >
// DefaultHost. After the cascade picks a raw value, the sentinel "tailscale"
// (case-insensitive) is replaced by this machine's Tailscale 100.x IP; when none
// is found (Tailscale down or not installed) it falls back to DefaultHost with a
// warning so the local UI still serves. Any other value is used verbatim as a
// TCP host; an unroutable or unparseable address surfaces only when the server
// tries to bind it, where a failed extra listener is a warning (loopback still
// serves).
func resolveHost() {
	Host = DefaultHost
	if v := strings.TrimSpace(AppConfig.Host); v != "" {
		Host = v
	}
	if v := strings.TrimSpace(os.Getenv("MODELSDB_HOST")); v != "" {
		Host = v
	}
	if v := strings.TrimSpace(flagHost); v != "" {
		Host = v
	}
	if strings.EqualFold(Host, "tailscale") {
		if ip, ok := tailscaleIP(); ok {
			Host = ip
		} else {
			log.Printf("Warning: host \"tailscale\" is set but no Tailscale 100.x address was found (Tailscale down or not installed); binding loopback only")
			Host = DefaultHost
		}
	}
}

// resolveAllowPublic resolves whether the server may bind a wildcard or public
// address, with the same first-hit-wins cascade as the other settings:
// --allow-public flag > MODELSDB_ALLOW_PUBLIC env > config.json "allow_public" >
// false. Off by default so a public bind is refused: the server has no auth.
func resolveAllowPublic() {
	AllowPublic = AppConfig.AllowPublic
	if v := strings.TrimSpace(os.Getenv("MODELSDB_ALLOW_PUBLIC")); v != "" {
		AllowPublic = isTruthy(v)
	}
	if flagAllowPublic {
		AllowPublic = true
	}
}

// isTruthy reports whether v is one of the accepted true words ("1", "true",
// "yes"), case-insensitive; anything else (including "0"/"false") is false.
func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// isTailscaleIP reports whether ip falls in the Tailscale/CGNAT range
// 100.64.0.0/10, the address block Tailscale assigns to a machine's tailscale0
// interface.
func isTailscaleIP(ip net.IP) bool {
	_, cgnat, err := net.ParseCIDR("100.64.0.0/10")
	if err != nil {
		return false
	}
	return cgnat.Contains(ip)
}

// tailscaleIP returns this machine's first IPv4 address in the Tailscale/CGNAT
// range 100.64.0.0/10 and true when one is present. It returns ("", false) when
// Tailscale is down or not installed (no such address on any interface).
func tailscaleIP() (string, bool) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", false
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip4 := ip.To4(); ip4 != nil && isTailscaleIP(ip4) {
			return ip4.String(), true
		}
	}
	return "", false
}

// BindExposure classifies a resolved bind host by how widely it exposes the
// server, returning one of "loopback", "private", "tailscale", "wildcard", or
// "public". The server has no authentication, so the app layer refuses a
// "public" or "wildcard" bind unless AllowPublic is set. By the time this runs
// the "tailscale" sentinel has already been replaced by a concrete 100.x IP (or
// fell back to loopback), so it never sees the literal word.
//
// A hostname (not an IP literal) is resolved: it counts as "private" only when
// every resolved address is loopback/private/tailscale; a lookup failure or any
// public address classifies it "public" (fail safe).
func BindExposure(host string) string {
	h := strings.TrimSpace(host)
	switch h {
	case "", "127.0.0.1", "localhost", "::1":
		return "loopback"
	case "0.0.0.0", "::":
		return "wildcard"
	}
	if ip := net.ParseIP(h); ip != nil {
		return classifyIP(ip)
	}
	ips, err := net.LookupIP(h)
	if err != nil || len(ips) == 0 {
		return "public"
	}
	for _, ip := range ips {
		switch classifyIP(ip) {
		case "public", "wildcard":
			return "public"
		}
	}
	return "private"
}

// classifyIP classifies a single IP literal as "tailscale", "loopback",
// "private", or "public". IsPrivate covers RFC 1918 IPv4 and unique-local IPv6
// (fc00::/7); IsLinkLocalUnicast covers 169.254/16 and fe80::/10.
func classifyIP(ip net.IP) string {
	switch {
	case isTailscaleIP(ip):
		return "tailscale"
	case ip.IsLoopback():
		return "loopback"
	case ip.IsPrivate() || ip.IsLinkLocalUnicast():
		return "private"
	default:
		return "public"
	}
}

func finalizePaths() {
	DbFile = filepath.Join(DataDir, "modelsdb.db")
	CuratedJsonFile = filepath.Join(DataDir, "curated.json")
	BackupsDir = filepath.Join(DataDir, ".backups")
	SettingsJsonFile = filepath.Join(ConfigDir, "settings.json")
	ConfigFile = filepath.Join(ConfigDir, configBase)
	LogsDir = filepath.Join(CacheDir, "logs")
	PidFile = filepath.Join(CacheDir, "modelsdb.pid")
	UpdateCacheFile = filepath.Join(CacheDir, "update-check.json")
}

func makeDirs() error {
	for _, d := range []string{ConfigDir, DataDir, CacheDir, LogsDir} {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// Package paths holds shared configuration: the loopback port, the resolved
// on-disk locations (config / data / cache), and the per-fork upstream repo.
//
// The directory and file path variables below are EMPTY until startup: they are
// populated by ResolvePaths (see paths.go), which runs in bootstrap before the
// DB is opened. Nothing here is baked in at build time, so the binary is
// portable across machines.
package paths

const (
	// DefaultPort is the port bound when nothing overrides it.
	DefaultPort = 8122

	// DefaultHost is the bind address used when nothing overrides it: loopback,
	// so a fresh install is reachable only from the same machine. Set "host" in
	// config.jsonc to also serve on another address: a Tailscale 100.x IP, any
	// private/LAN IP, or the sentinel "tailscale" (auto-detect this machine's
	// Tailscale 100.x IP).
	DefaultHost = "127.0.0.1"

	// appName is the per-OS application directory leaf (e.g. ~/.config/modelsdb,
	// %LOCALAPPDATA%\modelsdb, ~/Library/Application Support/modelsdb). It names a
	// folder only; the product name shown in the UI, logs, and docs stays
	// "ModelsDB".
	appName = "modelsdb"

	// legacyAppName is the capitalized directory leaf older builds used. Startup
	// migrates such a directory to appName once (see migrateLegacyDir), so an
	// existing install's data is not orphaned when the leaf lowercases.
	legacyAppName = "ModelsDB"

	// configFileName is the CANONICAL config file: the name written on first run and
	// used when none exists yet. On read the app also accepts a legacy "config.json"
	// of the same shape (see configNames). It is JSONC: comments and trailing commas
	// are allowed (see stripJSONC), so it can be self-documented.
	configFileName = "config.jsonc"

	// configHeader is the comment block SaveConfigFile writes above the JSON body,
	// documenting every (optional) option. config.json is JSONC, so these survive
	// as comments. Keep ASCII-only.
	configHeader = `// ModelsDB configuration (JSONC: // and /* */ comments and trailing commas are
// allowed). Every field below is OPTIONAL - delete one to use its default. Paths
// are relative to THIS file's folder unless absolute ("." means this folder).
//
//   "port": 8122,            // port the web UI + API bind (1-65535)
//   "host": "127.0.0.1",     // bind address. Loopback (default) = same machine
//                            // only. A LAN/VPN IP (e.g. a Tailscale 100.x) makes
//                            // it reachable there - there is NO auth, so anyone
//                            // who can route to that address has full read/write.
//                            // "tailscale" auto-detects this machine's Tailscale
//                            // 100.x IP. "0.0.0.0" binds every interface (widest
//                            // exposure) and requires "allow_public": true.
//   "allow_public": true,    // permit binding 0.0.0.0 or any public address.
//                            // OFF by default: a public bind is refused because
//                            // the server has no authentication.
//   "data_dir": ".",         // holds modelsdb.db (source of truth) + curated.json
//   "cache_dir": ".",        // holds logs/, the PID file, update-check.json
//   "portable": true,        // shortcut: put data + cache both in this folder
//   "upstream": { "owner": "DavidTimar1", "repo": "ModelsDB", "branch": "main" },
//                            // a fork's catalog + update-check source
//
`

	// MaxBackupsToKeep caps how many pre-migration DB backups are retained in
	// the data dir's .backups folder.
	MaxBackupsToKeep = 30
)

// Port is the port the server binds. Resolved once at startup (first hit wins):
// --port flag, MODELSDB_PORT env, config.json "port", else DefaultPort. It never
// auto-moves at runtime.
var Port = DefaultPort

// Host is the bind address the server listens on, alongside loopback. Resolved
// once at startup (first hit wins): --host flag, MODELSDB_HOST env, config.json
// "host", else DefaultHost. The server always binds loopback too, so local
// tooling and the single-instance port probe keep working regardless of Host.
// The sentinel "tailscale" is replaced during resolution by this machine's
// Tailscale 100.x IP (or falls back to DefaultHost when none is found).
var Host = DefaultHost

// AllowPublic permits binding a wildcard (0.0.0.0 / ::) or public address.
// Resolved once at startup (first hit wins): --allow-public flag,
// MODELSDB_ALLOW_PUBLIC env, config.json "allow_public", else false. It is off
// by default so a public bind is refused: the server has no authentication.
var AllowPublic bool

// Resolved absolute locations, set by ResolvePaths at startup.
var (
	ConfigDir string // holds config.json + settings.json
	DataDir   string // holds modelsdb.db, curated.json, .backups/
	CacheDir  string // holds logs/, modelsdb.pid, update-check.json
	LogsDir   string // = CacheDir/logs
	DirName   string // the app/working directory, for logging only

	DbFile           string // SQLite source of truth - ALL model data, objective + personal (DataDir)
	CuratedJsonFile  string // runtime export of the OBJECTIVE catalog subset (DataDir); the PUBLISHED catalog is embedded (internal/seedcatalog)
	SettingsJsonFile string // UI view/colour preferences (ConfigDir)
	ConfigFile       string // the config.json pointer (ConfigDir)
	BackupsDir       string // pre-migration DB snapshots (DataDir/.backups)
	PidFile          string // single-instance PID file (CacheDir)
	UpdateCacheFile  string // cached update-check result (CacheDir)

	ExePath string
)

// ActiveUpstream is the GitHub repo this build pulls its curated catalog and
// update notifications from. It defaults to the canonical repo and is overridable
// per fork via config.json. Set by ResolvePaths/applyUpstream.
var ActiveUpstream = defaultUpstream()

// AppConfig is the loaded config.json (zero value when none exists). It carries
// the first-run state and is persisted via SaveConfigFile.
var AppConfig fileConfig

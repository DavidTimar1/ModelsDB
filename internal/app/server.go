// Package app handles server logic and functionalities.
package app

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/selfupdate"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

//go:embed static/*
var staticFS embed.FS

//go:embed index.html
var indexHTML []byte

// resolvePaths resolves on-disk locations (config/data/cache and the files under
// them). It runs once at the start of Run, AFTER global flags are parsed, so
// --config-dir/--data-dir/etc. take effect. It is quiet: it does NOT wire the log
// file, so process-control commands (menu, stop, status) keep clean terminal
// output. It does NOT open the database: opening (and any schema migration) is a
// separate step that, on the server path, must run only AFTER the single-instance
// takeover has drained any previous process off the DB file - so two live
// processes never migrate the same DB at once.
func resolvePaths() error {
	if err := paths.ResolvePaths(); err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	return nil
}

// loggingStarted guards startLogging so the log file and the startup banner are
// wired at most once, whichever entry point (server, a data subcommand, or the
// menu's foreground action) reaches it first.
var loggingStarted bool

// startLogging wires the rotating log file and prints the startup banner. It is
// called by paths that actually do work worth logging (the server and the data
// subcommands), not by the quiet control commands.
func startLogging() {
	if loggingStarted {
		return
	}
	loggingStarted = true
	setupLogging()
	log.Printf("=== ModelsDB %s starting (pid %d) ===", shared.Version, os.Getpid())
}

// openAndSeedDB opens the SQLite database (running any pending schema
// migrations) and seeds an empty DB from curated.json. A populated DB is the
// source of truth and is left as-is. The DB carries all model data (objective +
// personal); nothing is loaded from or written to a personal file.
func openAndSeedDB() error {
	if err := dbcore.OpenDB(paths.DbFile); err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	if err := store.EnsureSeeded(); err != nil {
		log.Printf("Warning: DB seed failed: %v", err)
	}
	return nil
}

// setupLogging tees all log output to a size-rotated file under the cache dir so
// errors survive the console window closing, bounded to ~8 MB (2 MB x up to 4).
func setupLogging() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	if rw, err := shared.NewRotatingWriter(filepath.Join(paths.LogsDir, "modelsdb.log"), 2<<20, 3); err == nil {
		log.SetOutput(io.MultiWriter(os.Stderr, rw))
	}
}

// StartServer is exported func.
func StartServer() {

	// Wire the log file + banner now (idempotent): the server may be reached
	// directly, or via the menu's foreground action after a quiet path resolve.
	startLogging()

	// Refuse a public or wildcard bind unless explicitly allowed. The server has
	// no authentication, so binding a routable public address would expose full
	// read/write to anyone. A Tailscale or private address is the safe way to
	// reach it off this machine.
	if !paths.AllowPublic {
		if exp := paths.BindExposure(paths.Host); exp == "public" || exp == "wildcard" {
			shared.Fatalf("Refusing to bind public address %q: ModelsDB has no authentication. Use a Tailscale or private address (host: \"tailscale\"), or pass --allow-public to override.", paths.Host)
		}
	}

	// Remove any leftover binary a prior in-place self-update left aside (a
	// "<exe>.old" on Windows; a no-op elsewhere).
	selfupdate.CleanupOnStartup()

	// Single instance: stop any previously running instance and wait for it to
	// release the port (and, with it, the DB file), THEN open the DB. Two
	// instances must never run at once, and the DB must never be opened or
	// migrated while a previous process still holds it - which matters most on a
	// Windows self-update restart, where the new binary is launched while the old
	// process is still exiting.
	shared.KillPreviousInstance()
	shared.WriteInstance()
	log.Printf("PID %d (pidfile: %s)", os.Getpid(), shared.PidFilePath())

	// Open the DB (and run any migration) only now, after the takeover above.
	if err := openAndSeedDB(); err != nil {
		shared.Fatalf("startup failed: %v", err)
	}

	// Auto-update models on startup. A failed update must NOT take down the
	// server: log a warning and keep serving the existing data. This refresh
	// updates the DB but does NOT export the durable file (FetchModels no longer
	// exports) - startup must never overwrite a curated.json the user may have
	// restored. New data reaches the file on the next in-app save or the
	// "Update models" button.
	if err := catalog.FetchModels(); err != nil {
		log.Printf("Warning: startup model update failed, serving existing data: %v", err)
	}

	// Materialize the durable files on a fresh install (create only if missing);
	// this never overwrites an existing file the user placed there.
	store.ExportMissing()

	// Notify-only update check: poll the upstream repo's releases in the
	// background (startup + daily). Never downloads or replaces the binary.
	startUpdateChecker()

	// Print startup info
	log.Printf("=== ModelsDB Server (%s) ===", shared.Version)
	log.Printf("Executable:  %s", paths.ExePath)
	log.Printf("Config dir:  %s", paths.ConfigDir)
	log.Printf("Data dir:    %s", paths.DataDir)
	log.Printf("Cache dir:   %s", paths.CacheDir)
	log.Printf("  database:      %s", paths.DbFile)
	log.Printf("  curated.json:  %s", paths.CuratedJsonFile)
	log.Printf("  settings.json: %s", paths.SettingsJsonFile)
	if n, err := dbcore.CountModels(); err == nil {
		log.Printf("Models in DB: %d", n)
	}

	// Serve embedded static files. Embedded files carry a zero modtime, so the
	// FileServer sends no Last-Modified/ETag validator; without one a browser may
	// keep serving a cached app.js/style.css after the binary is updated. Send
	// Cache-Control: no-cache so the browser revalidates (and, lacking a
	// validator, refetches) every load - the assets are tiny and local, so a
	// fresh fetch each time is cheap and guarantees an update is seen on reload.
	staticSubFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		shared.Fatal(err)
	}
	staticServer := http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS)))
	http.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		staticServer.ServeHTTP(w, r)
	}))

	// Serve embedded index.html
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexHTML)
			return
		}
		http.NotFound(w, r)
	})

	http.HandleFunc("/api", handleAPIGuide)
	http.HandleFunc("/api/models", handleGetModels)
	http.HandleFunc("/api/save", handleSaveModels)
	http.HandleFunc("/api/settings", handleSettings)
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/paths", handlePaths)
	http.HandleFunc("/api/update", handleUpdate)
	http.HandleFunc("/api/update/status", handleUpdateStatus)
	http.HandleFunc("/api/update-check", handleUpdateCheck)
	http.HandleFunc("/api/app-update/apply", handleAppUpdateApply)
	http.HandleFunc("/api/app-update/status", handleAppUpdateStatus)
	http.HandleFunc("/api/app-update/restart", handleAppUpdateRestart)

	// Bind addresses: loopback is always first (local tooling and the
	// single-instance port probe depend on it), then the configured host when it
	// is a specific non-loopback address (e.g. a Tailscale 100.x IP).
	addrs := bindAddrs(paths.Host, paths.Port)

	// The first address is primary and must bind. Any previous ModelsDB instance
	// was already stopped above; if the port is still taken, a foreign process
	// holds it - fail with a clear error rather than silently moving to another port.
	primary, err := net.Listen("tcp", addrs[0])
	if err != nil {
		shared.Fatalf("Port %d is already in use by another process. ModelsDB uses this fixed port - free it and try again. (%v)", paths.Port, err)
	}
	log.Printf("Serving at http://%s", addrs[0])

	// Additional addresses are best-effort: an interface that is down or an
	// address not local to this machine must not stop local serving. The server
	// has no authentication, so a non-loopback bind exposes full read/write to
	// anyone who can route to it - hence the explicit log line.
	for _, addr := range addrs[1:] {
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			log.Printf("Warning: not serving on %s: %v", addr, lerr)
			continue
		}
		log.Printf("Also serving at http://%s (no auth - anyone who can route here has full read/write)", addr)
		go func() { shared.Fatal(http.Serve(ln, nil)) }()
	}
	shared.Fatal(http.Serve(primary, nil))
}

// bindAddrs returns the TCP addresses the server listens on for the given host
// and port. Loopback is always included so local tooling and the single-instance
// port probe keep working; a specific non-loopback host (e.g. a Tailscale 100.x
// IP) is added so the UI is reachable there too. A wildcard host (0.0.0.0 / ::)
// already covers loopback, so it binds alone. A public or wildcard host is
// gated earlier by paths.BindExposure + paths.AllowPublic, so by the time this
// runs an unallowed public bind has already been refused.
func bindAddrs(host string, port int) []string {
	p := strconv.Itoa(port)
	switch host {
	case "", "127.0.0.1", "localhost", "::1":
		return []string{net.JoinHostPort("127.0.0.1", p)}
	case "0.0.0.0", "::":
		return []string{net.JoinHostPort(host, p)}
	default:
		return []string{net.JoinHostPort("127.0.0.1", p), net.JoinHostPort(host, p)}
	}
}

// The HTTP server: the single list of routes, the gate every request passes, and startup.

package app

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/selfupdate"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// staticFS and staticFSSubdir are declared in embed_dev.go (default build:
// embeds static/* unminified) or embed_prod.go (`-tags minified`: embeds
// static-min/*, the minified copy build.sh generates) - see those files.

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

// openMigratedDB opens the database and runs any pending schema migrations, and
// writes nothing else. It is what a command that only READS the data opens: the
// column seeding and source declarations openDB adds are writes inside a
// transaction that reads first, and SQLite answers SQLITE_BUSY at once to such a
// transaction when a server holds the write lock (its start-up refresh does, for
// minutes), without waiting out the busy timeout.
func openMigratedDB() error {
	if err := dbcore.OpenDB(paths.DbFile); err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	return nil
}

// openDB opens the SQLite database and runs any pending schema migrations. The
// database is the only place model data lives, objective and personal alike, so
// there is nothing else to load: an empty database stays empty until the catalog
// is fetched from OpenRouter.
func openDB() error {
	if err := openMigratedDB(); err != nil {
		return err
	}
	// The columns the app ships are put in AFTER the migrations, which may have
	// reshaped the tables they land in. Seeding is additive and changes nothing
	// that is already there, so it runs on every start rather than being
	// remembered as done.
	created, err := store.SeedShippedColumns()
	if err != nil {
		return fmt.Errorf("add the columns this app ships: %w", err)
	}
	if len(created) > 0 {
		log.Printf("Added %d column(s) this app ships", len(created))
	}
	// Which sources a column declares is code, not stored data, so it is
	// reapplied at every start; a column it cannot safely rewrite is logged and
	// skipped rather than stopping the server.
	rewritten, err := store.DeclareShippedSources()
	if err != nil {
		return fmt.Errorf("declare the sources of the columns: %w", err)
	}
	log.Printf("Declared the sources of %d column(s)", rewritten)
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

// apiRoute is one HTTP pattern this server answers, paired with the handler that
// answers it. serverRoutes below is the single list of them, which is what lets a
// test range over the REAL registrations instead of re-deriving them by hand:
// TestEveryRegisteredRouteIsDocumented fails when a route has no heading of its
// own in the served API guide (apiGuide, in query.go).
//
// A pattern with no trailing slash matches that exact path and nothing below it,
// which is what lets /api/models/variations and /api/models/variations/files be
// two separate handlers rather than one dispatching on the rest of the path.
//
// undocumented marks a route the guide deliberately says nothing about, because
// no API caller ever calls it. Only the two that serve the browser app itself
// qualify: the page at "/" and its embedded assets under "/static/". Every /api
// route is documented, with no exceptions.
type apiRoute struct {
	pattern      string
	handler      http.HandlerFunc
	undocumented bool
}

// serverRoutes builds the complete route list, including the static-asset
// handler - which needs the embedded sub-filesystem, the one part of the wiring
// that can fail. It REGISTERS nothing, so a test may call it as often as it
// likes without touching the default request multiplexer.
func serverRoutes() ([]apiRoute, error) {
	// Serve embedded static files. Embedded files carry a zero modtime, so the
	// FileServer sends no Last-Modified/ETag validator; without one a browser may
	// keep serving a cached app.js/style.css after the binary is updated. Send
	// Cache-Control: no-cache so the browser revalidates (and, lacking a
	// validator, refetches) every load - the assets are tiny and local, so a
	// fresh fetch each time is cheap and guarantees an update is seen on reload.
	staticSubFS, err := fs.Sub(staticFS, staticFSSubdir)
	if err != nil {
		return nil, err
	}
	staticServer := http.StripPrefix("/static/", http.FileServer(http.FS(staticSubFS)))

	return []apiRoute{
		{pattern: "/static/", undocumented: true, handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			staticServer.ServeHTTP(w, r)
		}},
		// A browser asks for /favicon.ico on any page that declares no icon, and
		// GET /api is markdown with nowhere to declare one, so the same icon the
		// page links is answered here rather than as a 404 in the console.
		{pattern: "/favicon.ico", undocumented: true, handler: func(w http.ResponseWriter, r *http.Request) {
			icon := r.Clone(r.Context())
			icon.URL.Path = "/static/favicon.ico"
			w.Header().Set("Cache-Control", "no-cache")
			staticServer.ServeHTTP(w, icon)
		}},
		// The embedded single-page frontend, and a 404 for every other unmatched path.
		{pattern: "/", undocumented: true, handler: func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(indexHTML)
				return
			}
			http.NotFound(w, r)
		}},

		{pattern: "/api", handler: handleAPIGuide},
		{pattern: "/api/guide/update-database-manually", handler: handleManualUpdateGuide},
		{pattern: "/api/models", handler: handleGetModels},
		{pattern: "/api/models/similar", handler: handleSimilarModels},
		{pattern: "/api/models/{name}/answers/{column}", handler: handleModelAnswers},
		{pattern: "/api/save", handler: handleSaveModels},
		{pattern: "/api/models/add", handler: handleAddModel},
		{pattern: "/api/models/brief", handler: handleModelBrief},
		{pattern: "/api/models/import", handler: handleImportHF},
		{pattern: "/api/columns", handler: handleColumns},
		{pattern: "/api/columns/values", handler: handleColumnValues},
		{pattern: "/api/columns/answers", handler: handleColumnAnswers},
		{pattern: "/api/import-fields", handler: handleImportFields},
		{pattern: "/api/models/delete", handler: handleDeleteModel},
		{pattern: "/api/models/hide", handler: handleHideModel},
		{pattern: "/api/models/variations", handler: handleVariations},
		{pattern: "/api/models/variations/files", handler: handleVariationFiles},
		{pattern: "/api/models/variations/existing", handler: handleFileAsVariation},
		{pattern: "/api/models/variations/unfile", handler: handleUnfileVariation},
		{pattern: "/api/settings", handler: handleSettings},
		{pattern: "/api/settings/ack-notice", handler: handleSettingsAckNotice},
		{pattern: "/api/health", handler: handleHealth},
		{pattern: "/api/paths", handler: handlePaths},
		{pattern: "/api/update", handler: handleUpdate},
		{pattern: "/api/update/status", handler: handleUpdateStatus},
		{pattern: "/api/update-check", handler: handleUpdateCheck},
		{pattern: "/api/app-update/apply", handler: handleAppUpdateApply},
		{pattern: "/api/app-update/status", handler: handleAppUpdateStatus},
		{pattern: "/api/app-update/restart", handler: handleAppUpdateRestart},
		{pattern: "/api/backup/validate", handler: handleBackupValidate},
		{pattern: "/api/backup/run", handler: handleBackupRun},
		{pattern: "/api/backup/status", handler: handleBackupStatus},
	}, nil
}

// StartServer binds the port, opens the database, refreshes the catalog and
// serves until a listener fails.
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
	// release the port (and, with it, the DB file). Two instances must never run
	// at once, and the DB must never be opened or migrated while a previous
	// process still holds it - which matters most on a Windows self-update
	// restart, where the new binary is launched while the old process is still
	// exiting.
	shared.KillPreviousInstance()

	// Take the port BEFORE opening the database, and record this process only
	// once the port is its own. The port is what every other instance reads to
	// decide whether this data dir is taken, so holding the database without
	// holding the port would let a second start conclude nothing is running and
	// open the same file. Writing the record after the bind is the other half:
	// a start that cannot bind exits here, leaving the record of the healthy
	// server that holds the port exactly as it found it.
	addrs := bindAddrs(paths.Host, paths.Port)
	primary, err := net.Listen("tcp", addrs[0])
	if err != nil {
		shared.Fatalf("Port %d is already in use by another process. ModelsDB uses this fixed port - free it and try again. (%v)", paths.Port, err)
	}
	recordServed(addrs[0])
	shared.WriteInstance()
	log.Printf("PID %d (pidfile: %s)", os.Getpid(), shared.PidFilePath())

	// Answer on the port from the moment it is bound. Until the rest of startup
	// finishes, every request gets the short "starting" reply below rather than
	// hanging in the accept queue, so a stop, a status, or the browser waiting
	// out a self-update restart always gets a straight answer - including this
	// process's id, which is what lets them act on it.
	serveErr := make(chan error, len(addrs))
	go func() { serveErr <- newHTTPServer(gatedHandler()).Serve(primary) }()

	// Additional addresses are best-effort: an interface that is down or an
	// address not local to this machine must not stop local serving. The server
	// has no authentication, so a non-loopback bind exposes full read/write to
	// anyone who can route to it - hence the explicit log line. Only an address
	// that BOUND is recorded as served, so GET /api/health never names one that
	// this process is not answering on.
	for _, addr := range addrs[1:] {
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			log.Printf("Warning: not serving on %s: %v", addr, lerr)
			continue
		}
		recordServed(addr)
		log.Printf("Also serving at http://%s (no auth - anyone who can route here has full read/write)", addr)
		go func(ln net.Listener) { serveErr <- newHTTPServer(gatedHandler()).Serve(ln) }(ln)
	}

	// Open the DB (and run any migration) only now, with the port already held.
	if err := openDB(); err != nil {
		shared.Fatalf("startup failed: %v", err)
	}

	// Auto-update models on startup. A failed update must NOT take down the
	// server: it is logged and the existing data is served. The record of the
	// last run is loaded first so this run continues the count.
	loadModelUpdateCache()
	runStartupModelUpdate()

	// Notify-only update check: poll the upstream repo's releases in the
	// background (startup + daily). Never downloads or replaces the binary.
	startUpdateChecker()

	// Opt-in daily backup: writes into a folder the user chose, once every 24
	// hours, only when the database has actually changed since the last one.
	// Off by default; attemptScheduledBackup (not this call) is what decides
	// whether the feature is even on.
	startBackupScheduler()

	// Opt-in daily model update: the same refresh as the Update models button,
	// started by a timer once a day while the switch in Info is on.
	startModelUpdateScheduler()

	// Print startup info
	log.Printf("=== ModelsDB Server (%s) ===", shared.Version)
	log.Printf("Executable:  %s", paths.ExePath)
	log.Printf("Config dir:  %s", paths.ConfigDir)
	log.Printf("Data dir:    %s", paths.DataDir)
	log.Printf("Cache dir:   %s", paths.CacheDir)
	log.Printf("  database:      %s", paths.DbFile)
	log.Printf("  config.jsonc:  %s", paths.ConfigFile)
	if n, err := dbcore.CountModels(); err == nil {
		log.Printf("Models in DB: %d", n)
	}

	// Register every route from the one list that defines them. The order does
	// not matter: the default multiplexer keys its handlers on the pattern, and
	// the longest matching pattern wins per request whatever order they arrived in.
	routes, err := serverRoutes()
	if err != nil {
		shared.Fatal(err)
	}
	for _, rt := range routes {
		http.HandleFunc(rt.pattern, rt.handler)
	}

	// Everything a request needs is now in place, so open the gate: from here the
	// listeners bound above serve the real application.
	startupComplete.Store(true)
	log.Printf("Serving at http://%s", addrs[0])

	// Block on the listeners. http.Serve returns only on a failure, and the first
	// failure ends the process - a half-served instance would still hold the port
	// and the database while answering nothing.
	shared.Fatal(<-serveErr)
}

// startupComplete reports whether this server can answer real requests: the
// database is open and every route is registered. The port is bound before
// either is true - taking the port is what stops a second instance opening the
// same database - so until this flips, gatedHandler answers instead.
var startupComplete atomic.Bool

// gatedHandler is what the listeners serve, and it holds the gates every request
// passes before any handler sees it. Before startup finishes it answers with the
// "starting" reply, so no request can reach a route list that is still being
// built or a database that is not open yet - except a browser opening the page
// itself, which gets startingPageHTML instead: a human staring at raw JSON has no
// way to know to keep reloading, while the page polls for them. After startup
// finishes gatedHandler applies hostAllowed and originAllowed, which sit here
// rather than in a handler so no route can be missed and no body is read from a
// request that will be refused, and caps the size of every body it lets through.
// Every answer carries the framing headers.
func gatedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if !startupComplete.Load() {
			if r.URL.Path == "/" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				writeStartingPage(w)
				return
			}
			writeStarting(w)
			return
		}
		if !hostAllowed(r) {
			writeForeignHost(w, r)
			return
		}
		if !originAllowed(r) {
			writeForeignOrigin(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		}
		http.DefaultServeMux.ServeHTTP(w, r)
	})
}

// originAllowed reports whether a request is allowed to reach a handler, once
// hostAllowed has accepted its Host. This server has no authentication, so
// without this check any page the reader has open in the same browser could drive
// every write route here, using the reader's own browser to reach an address only
// the reader can reach. A browser states the page a request came from in Origin
// and cannot be talked out of it. A command-line caller sends no Origin at all,
// which is why an absent header is allowed: that is what keeps curl, scripts and
// assistants working.
//
// THAT BREAKAGE IS AN HTTPS REVERSE PROXY in front of this app. The browser
// sends "Origin: https://name" while the proxy forwards "Host: name" with no
// port, so the two no longer match; config.jsonc "allowed_origins"
// (paths.AllowedOrigins, read once at startup - never over HTTP, see
// resolveAllowedOrigins in internal/paths/paths.go) is the operator naming
// that proxy address by hand, checked below as a second way to pass.
func originAllowed(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	sent := r.Header.Values("Origin")
	if len(sent) == 0 {
		return true
	}
	if len(sent) > 1 {
		return false
	}
	u, err := url.Parse(sent[0])
	if err != nil || u.Host == "" {
		return false
	}
	// The port stays in the comparison, because another service on a different
	// port of this same host is a different origin and must not write here.
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	return paths.OriginListed(sent[0])
}

// writeForeignOrigin refuses a write that came from a page on another address.
// The offending Origin is never echoed into the answer: it is text whoever
// mounted the attack chose, and repeating it puts that text back on a screen.
func writeForeignOrigin(w http.ResponseWriter, r *http.Request) {
	log.Printf("REFUSED [%s %q] write from a web page on another address", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte("Refused: this write arrived from a web page served at another address. ModelsDB takes writes only from its own page.\n" +
		"Calling from a command line: send NO Origin header at all. curl sends none unless you add one.\n" +
		fmt.Sprintf("Calling from another address you control: list it under \"allowed_origins\" in %s (currently %d accepted) and restart.\n", paths.ConfigFile, len(paths.AllowedOrigins))))
}

// writeStarting answers a request that arrived while the server was still
// starting. The status is 503, so anything waiting for this server to come up
// (the browser's reconnect poll after a self-update restart) correctly reads it
// as "not yet"; the body is the same identity block GET /api/health returns, so
// a stop or a status can still learn which process holds the port.
func writeStarting(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(instanceInfo("starting"))
}

// startingPageHTML is served in place of indexHTML for a browser opening "/"
// during the startup gate. It is a Go string constant rather than a second
// embedded file because it needs no file of its own: it is self-contained (no
// /static/ asset, no external resource) since serverRoutes() - which wires
// /static/ - and startupComplete both come later in StartServer, so nothing
// this page could reference exists yet.
const startingPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>ModelsDB is starting</title>
<style>
body { font-family: sans-serif; background: #111; color: #eee; display: flex;
  align-items: center; justify-content: center; height: 100vh; margin: 0; }
main { text-align: center; max-width: 32em; padding: 1em; }
#retry-message { display: none; margin-top: 1em; }
</style>
</head>
<body>
<main>
<h1>ModelsDB is starting</h1>
<p>The model catalog is being fetched from OpenRouter. This page will open on its own once that is done.</p>
<p id="retry-message">This is taking longer than expected. Reload the page by hand to try again.</p>
</main>
<script>
(function () {
  var tries = 0;
  var maxTries = 300;
  var timer = setInterval(function () {
    tries++;
    if (tries > maxTries) {
      clearInterval(timer);
      document.getElementById('retry-message').style.display = 'block';
      return;
    }
    fetch('/api/health', { cache: 'no-store' }).then(function (r) {
      if (r.ok) {
        location.reload();
      }
    }).catch(function () {});
  }, 1000);
})();
</script>
</body>
</html>
`

// writeStartingPage answers a browser's GET/HEAD "/" during the startup gate
// with a small human-readable page instead of the JSON writeStarting sends
// everywhere else, so a person watching the tab sees words instead of raw
// JSON, and reloads on their own once the poll inside the page detects the
// server is up. Status and caching mirror writeStarting: 503 so nothing reads
// this as a real page load, no-cache so a retry always re-checks rather than
// replaying a cached "starting" response after the app comes up.
func writeStartingPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusServiceUnavailable)
	io.WriteString(w, startingPageHTML)
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

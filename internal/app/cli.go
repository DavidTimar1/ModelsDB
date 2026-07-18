// Package app handles cli logic and functionalities.
package app

import (
	"fmt"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"os"
)

// isKnownSubcommand reports whether arg names a CLI subcommand (as opposed to
// the default server run). Only these open the DB directly; the server path
// opens it AFTER the single-instance takeover (see StartServer).
func isKnownSubcommand(arg string) bool {
	switch arg {
	case "update", "collections", "pricing", "backups", "restore", "export":
		return true
	}
	return false
}

// Run parses global flags and resolves paths, then dispatches: a process-control
// command (menu/serve/start/stop/status), a data subcommand (which opens the DB
// directly), or - with no subcommand - the interactive menu on a terminal, else
// the server. The server opens the DB only after taking over any previous
// instance.
func Run() {
	defer shared.RecoverPanic() // keep an unexpected crash visible (pause + log)

	args, unknown := paths.ParseGlobalFlags(os.Args[1:]) // sets the --*-dir overrides, returns the rest and any unknown flags

	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}

	// Help, version, and unknown-flag handling run BEFORE any path resolution or
	// server start, so an informational or stray flag never touches the data,
	// config, or cache dirs and never takes over a running instance.
	if paths.HelpRequested() || cmd == "help" {
		printUsage()
		return
	}
	if paths.VersionRequested() {
		fmt.Println(shared.Version)
		return
	}
	if len(unknown) > 0 {
		fmt.Fprintf(os.Stderr, "modelsdb: unknown option %q\nRun 'modelsdb --help' for usage.\n", unknown[0])
		os.Exit(2)
	}

	if err := resolvePaths(); err != nil {
		shared.Fatalf("startup failed: %v", err)
	}

	// Process-control commands. These manage the server process and resolve paths
	// quietly (no log file / startup banner) so their terminal output is clean.
	switch cmd {
	case "menu":
		runMenu()
		return
	case "serve": // force the foreground server, bypassing the menu
		StartServer()
		return
	case "start": // start the server detached, in the background
		startBackground()
		return
	case "stop":
		stopBackground()
		return
	case "status":
		printStatus(false)
		return
	}

	if isKnownSubcommand(cmd) {
		startLogging()
		if err := openAndSeedDB(); err != nil {
			shared.Fatalf("startup failed: %v", err)
		}
		switch cmd {
		case "update":
			if err := catalog.FetchModels(); err != nil {
				shared.Fatal(err)
			}
			// FetchModels no longer exports (so the automatic startup refresh can't
			// clobber a restored file); this is an explicit user command, so persist.
			if err := store.ExportData(); err != nil {
				shared.Fatal(err)
			}
			os.Exit(0)
		case "collections":
			if err := catalog.RunCollections(); err != nil {
				shared.Fatal(err)
			}
			os.Exit(0)
		case "pricing":
			if err := catalog.RunPricing(); err != nil {
				shared.Fatal(err)
			}
			os.Exit(0)
		case "export":
			// Write the objective catalog (personal + unlisted excluded) to a file.
			// Publishing embeds it: `modelsdb export internal/seedcatalog/catalog.json`
			// from the repo root, then commit + build the release. With no path it
			// rewrites the data dir's curated.json.
			target := paths.CuratedJsonFile
			if len(args) >= 2 {
				target = args[1]
			}
			b, err := store.CuratedBytes()
			if err != nil {
				shared.Fatal(err)
			}
			if err := os.WriteFile(target, b, 0644); err != nil {
				shared.Fatal(err)
			}
			fmt.Printf("Exported curated catalog (%d bytes) to %s\n", len(b), target)
			os.Exit(0)
		case "backups":
			dbcore.ListBackups()
			os.Exit(0)
		case "restore":
			if len(args) < 2 {
				shared.Fatal("usage: modelsdb restore <backup-file>  (run 'modelsdb backups' to list)")
			}
			if err := dbcore.RestoreBackup(args[1]); err != nil {
				shared.Fatal(err)
			}
			os.Exit(0)
		}
	}

	// No subcommand. In an interactive terminal, offer the process-control menu;
	// otherwise (a pipe, a service, nohup with stdin redirected) start the server
	// directly, so existing non-interactive launches keep working unchanged.
	if cmd == "" {
		if isInteractive() {
			runMenu()
			return
		}
		StartServer()
		return
	}

	// cmd is a non-empty token that matched no command (a typo or a stray
	// argument). Reject it instead of falling through to start a server on the
	// default dirs, which would take over any running instance.
	fmt.Fprintf(os.Stderr, "modelsdb: unknown command %q\nRun 'modelsdb --help' for usage.\n", cmd)
	os.Exit(2)
}

// printUsage writes the command-line help to stdout: the usage line, the
// subcommands, and the global flags. It opens neither the database nor a
// listener.
func printUsage() {
	fmt.Printf(`ModelsDB %s - browse, rate, and annotate AI models.

Usage:
  modelsdb [global flags]            Launch: interactive menu on a terminal,
                                     otherwise the foreground server.
  modelsdb <command> [global flags]

Commands:
  serve          Run the web UI + API in the foreground.
  start          Start the server in the background.
  stop           Stop the background server.
  status         Report whether the server is running.
  update         Refresh the model catalog from OpenRouter.
  collections    Re-scrape OpenRouter collections into the catalog.
  pricing        Refresh pricing for collection-added models.
  backups        List the pre-migration database backups.
  restore <f>    Restore the database from backup file <f>.
  export [f]     Write the objective catalog to <f> (default: the data-dir curated.json).

Global flags (accepted before or after a command):
  --config-dir <dir>   Override the config directory.
  --data-dir <dir>     Override the data directory (holds modelsdb.db).
  --cache-dir <dir>    Override the cache directory (logs, PID file).
  --port <n>           Bind port (1-65535).
  --host <addr>        Extra bind address (an IP, or "tailscale").
  --portable           Put config, data, and cache in the executable's folder.
  --allow-public       Permit binding a wildcard or public address.
  -h, --help           Show this help and exit.
  -v, --version        Show the version and exit.
`, shared.Version)
}

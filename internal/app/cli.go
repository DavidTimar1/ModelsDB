// The command line: which subcommands exist and how each is dispatched.

package app

import (
	"fmt"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"os"
)

// isKnownSubcommand reports whether arg names a CLI subcommand (as opposed to
// the default server run). Only these open the DB directly; the server path
// opens it AFTER the single-instance takeover (see StartServer).
func isKnownSubcommand(arg string) bool {
	switch arg {
	case "update", "backup":
		return true
	}
	return false
}

// isNoDBSubcommand reports whether arg names a subcommand that must run with
// NO database open at all. A database below dbcore's schema floor is refused
// by dbcore.OpenDB before anything is read from it - so backups and restore
// are exactly the verbs a reader in that situation needs, and they cannot sit
// behind the same open that refuses them. backups only reads a directory
// listing; restore checks the candidate file's own version before touching
// the live database (see dbcore.RestoreBackup).
func isNoDBSubcommand(arg string) bool {
	switch arg {
	case "backups", "restore":
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
	// Ignore SIGPIPE before anything below can write to stdout or stderr, on
	// every path out of this function (menu, a subcommand, serve, or the bare
	// invocation) - a write racing a downstream reader that already closed must
	// never be able to kill this process. See sigpipe_unix.go.
	shared.IgnoreBrokenPipeSignal()

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
		if startBackground() {
			os.Exit(1)
		}
		return
	case "stop":
		stopBackground()
		return
	case "status":
		printStatus(false)
		return
	}

	if isNoDBSubcommand(cmd) {
		startLogging()
		switch cmd {
		case "backups":
			// Two mechanisms, opposite retention policies: the app's own
			// pre-migration snapshots (capped, pruned) and the user's own
			// destination-folder backups (never pruned). dbcore.ListBackups only
			// knows the first - it sits below the backup configuration in the
			// import DAG - so this prints a label around it, then adds the second
			// section itself. Neither needs the database open.
			fmt.Printf("Pre-migration backups (taken automatically before a schema migration; capped at %d files):\n", paths.MaxBackupsToKeep)
			dbcore.ListBackups()
			printConfiguredBackups()
			os.Exit(0)
		case "restore":
			if len(args) < 2 {
				shared.Fatal("usage: modelsdb restore <backup-file>  (run 'modelsdb backups' to list)")
			}
			refuseWhileServerRuns("restore")
			if err := dbcore.RestoreBackup(args[1]); err != nil {
				shared.Fatal(err)
			}
			os.Exit(0)
		}
	}

	if isKnownSubcommand(cmd) {
		startLogging()
		switch cmd {
		case "update":
			refuseWhileServerRuns("update")
		case "backup":
			// A copy made with VACUUM INTO only reads, so it may run beside a
			// server and opens the database through openMigratedDB, which writes
			// nothing but a pending migration. That migration must not run under
			// a server when this build is newer than the one running.
			refuseWhileServerRunsOldSchema("backup")
		}
		open := openDB
		if cmd == "backup" {
			open = openMigratedDB
		}
		if err := open(); err != nil {
			shared.Fatalf("startup failed: %v", err)
		}
		switch cmd {
		case "update":
			added, _, _, err := catalog.FetchModels()
			if err != nil {
				shared.Fatal(err)
			}
			fmt.Printf("%d model(s) added to the library by this update\n", added)
			os.Exit(0)
		case "backup":
			// One complete copy of the database - every model, every note, every
			// favourite, every value in a column of their own - into a folder the user
			// names.
			// The folder is required: where a backup lives is a decision about
			// somebody else's disk, and a default would put the only copy of the
			// data on the same disk as the original.
			if len(args) < 2 {
				shared.Fatal("usage: modelsdb backup <folder>  (writes one .db copy of your database into <folder>)")
			}
			dst, err := dbcore.BackupTo(args[1], dbcore.TimestampedBackup)
			if err != nil {
				shared.Fatal(err)
			}
			var size int64
			if info, serr := os.Stat(dst); serr == nil {
				size = info.Size()
			}
			fmt.Printf("Backed up the database (%d bytes) to %s\n", size, dst)
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
  backup <dir>   Copy the whole database into folder <dir> as one .db file.
  backups        List the pre-migration backups AND your own backup folder.
  restore <f>    Restore the database from backup file <f>.

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

// serverHoldingData finds a server this command must not run beside and returns
// the port it actually holds. It asks two places, because a server started with
// a different --port holds this data folder just as firmly:
//
//  1. The port this command resolved: this install's own server, or a process
//     that does not identify itself, which may be one.
//  2. The port the data folder's own instance record names, when that is another
//     port. It counts only while that port is held by a server that is this
//     install's; a record whose process is gone, or whose port belongs to
//     something else, holds nothing.
//
// A ModelsDB server for another data folder does not hold this one.
func serverHoldingData() (port int, held bool) {
	if h := shared.InspectPort(paths.Port); h.Bound && (!h.Known || h.Ours) {
		return paths.Port, true
	}
	if inst, ok := shared.ReadInstance(); ok && inst.Port > 0 && inst.Port != paths.Port {
		if h := shared.InspectPort(inst.Port); h.Bound && h.Known && h.Ours {
			return inst.Port, true
		}
	}
	return 0, false
}

// refuseWhileServerRuns stops a command that writes the database while a server
// has it open: the server's unwritten changes would be lost or the file
// corrupted. verb names the command in the message.
func refuseWhileServerRuns(verb string) {
	if port, held := serverHoldingData(); held {
		shared.Fatalf("modelsdb %s: a server is running on port %d and has the database open. Stop it with 'modelsdb stop', run this again, then start the server.", verb, port)
	}
}

// refuseWhileServerRunsOldSchema stops a command that opens the database
// normally while a server runs and the file still needs a schema migration: that
// migration must not run under the server.
func refuseWhileServerRunsOldSchema(verb string) {
	port, held := serverHoldingData()
	if !held {
		return
	}
	pending, err := dbcore.MigrationPending(paths.DbFile)
	if err != nil || pending {
		shared.Fatalf("modelsdb %s: a server is running on port %d and the database needs a schema migration (or its version could not be read). Stop the server with 'modelsdb stop', run this again, then start the server.", verb, port)
	}
}

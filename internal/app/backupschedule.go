// The opt-in daily backup scheduler, its shared backup helper,
// the destination validator, and the listing of the user's own backup folder.
//
// This is separate from the pre-migration snapshots in dbcore/migrate.go (a
// different mechanism with a different purpose - see dbcore.BackupTo and
// docs/architecture.md). It writes into a folder the USER chooses, on a
// 24-hour cadence, and only when the database has actually changed since the
// last successful backup.
//
// It never prunes, renames, or deletes anything in the user's folder except the
// single file that overwrite mode replaces.
//
// "DID ANYTHING CHANGE" IS ASKED IN TWO TIERS. The first is the two files on
// disk: size and modification time of modelsdb.db and its -wal sidecar,
// compared against the pair recorded at the last SUCCESSFUL backup. An absent
// -wal file (right after a checkpoint, or on a brand-new database) is a valid
// fingerprint value, not an error.
//
// The reader's view is rows in this database, so the file changes when somebody
// sorts a column, and a scheduler that stopped at the first tier would collect a
// backup a day of a catalog nobody had touched, into a folder it is forbidden to
// prune. So a CHANGED file is not proof: the second tier compares
// dbcore.ContentDigest, which reads every table except the one holding that
// view, against the digest recorded at the last successful backup.
//
// BOTH TIERS ERR TOWARD "CHANGED". A digest that cannot be computed means back
// up: an extra file costs disk and a missed backup costs the data.

package app

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const backupCheckInterval = 24 * time.Hour

// Outcomes recorded after an attempt, served to the modal and printed by the
// CLI. "" (empty) means no attempt has ever run yet.
const (
	backupOutcomeOK          = "ok"
	backupOutcomeUnchanged   = "unchanged"
	backupOutcomeInvalidDest = "invalid destination"
	backupOutcomeError       = "error"
)

// backupScheduleConfig is the user's own configuration for the daily backup:
// on/off, the destination folder, and the naming mode.
//
// IT IS NOT IN THE DATABASE, and that is the point. Where a copy of the database
// may be written is a fact about this machine, so it sits in config.jsonc beside
// data_dir: restoring somebody else's backup must never be able to repoint where
// the next one goes.
type backupScheduleConfig struct {
	Enabled   bool
	Dir       string
	Overwrite bool
}

// readBackupScheduleConfig answers the backup settings config.jsonc was read
// with, treating an absent or unreadable file as "disabled, no destination".
func readBackupScheduleConfig() backupScheduleConfig {
	enabled, dir, overwrite := paths.BackupSettings()
	return backupScheduleConfig{Enabled: enabled, Dir: dir, Overwrite: overwrite}
}

// dbFingerprint is (size, mtime) of modelsdb.db and its -wal sidecar. It is
// compared for DIFFERENCE, never for "newer than": the WAL's size recurs after
// every checkpoint, and a ">" comparison on mtime breaks if the clock ever
// moves backward. A file that does not exist contributes its zero value, which
// is itself a valid fingerprint (not an error) - normal right after a
// checkpoint, or on a brand-new database.
type dbFingerprint struct {
	DbSize     int64 `json:"db_size"`
	DbMtimeNs  int64 `json:"db_mtime_ns"`
	WalSize    int64 `json:"wal_size"`
	WalMtimeNs int64 `json:"wal_mtime_ns"`
}

func currentDBFingerprint() dbFingerprint {
	var fp dbFingerprint
	if info, err := os.Stat(paths.DbFile); err == nil {
		fp.DbSize = info.Size()
		fp.DbMtimeNs = info.ModTime().UnixNano()
	}
	if info, err := os.Stat(paths.DbFile + "-wal"); err == nil {
		fp.WalSize = info.Size()
		fp.WalMtimeNs = info.ModTime().UnixNano()
	}
	return fp
}

// backupScheduleState is the scheduler's own state: when it last attempted a
// backup, when one last actually succeeded, the fingerprint and content digest
// recorded at that success, and the outcome/error to show in the modal. It is
// SERVER-owned and lives in its own cache-dir file (see
// paths.BackupScheduleCacheFile), because it is a fact about this machine that
// is worthless anywhere else: a restored backup must not tell this install that
// it already backed up today.
//
// Two timestamps, because two different questions are asked of them.
// LastAttemptAt is when a backup was last TRIED by any route, the scheduler's
// and the "Back up now" button's alike, and it is what the Info dialog's status
// line reads. LastScheduledAt is the SCHEDULER's own reference point and nothing
// else reads it, so the 24-hour gate cannot be moved by a button press: taking a
// copy by hand must not postpone tomorrow's automatic one, which may well be
// going somewhere else.
type backupScheduleState struct {
	LastAttemptAt         string        `json:"last_attempt_at"`
	LastScheduledAt       string        `json:"last_scheduled_at"`
	LastBackupAt          string        `json:"last_backup_at"`
	HaveBackupFingerprint bool          `json:"have_backup_fingerprint"`
	LastBackupFingerprint dbFingerprint `json:"last_backup_fingerprint"`
	LastBackupDigest      string        `json:"last_backup_digest"`
	LastOutcome           string        `json:"last_outcome"`
	LastError             string        `json:"last_error"`
	LastPath              string        `json:"last_path"`
}

var (
	backupScheduleMu   sync.Mutex
	backupScheduleLast backupScheduleState
)

func loadBackupScheduleCache() {
	b, err := os.ReadFile(paths.BackupScheduleCacheFile)
	if err != nil {
		return
	}
	var st backupScheduleState
	if json.Unmarshal(b, &st) == nil {
		backupScheduleMu.Lock()
		backupScheduleLast = st
		backupScheduleMu.Unlock()
	}
}

func saveBackupScheduleCache(st backupScheduleState) {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(paths.CacheDir, 0755)
	if err := paths.WriteFileAtomic(paths.BackupScheduleCacheFile, b, 0644); err != nil {
		log.Printf("Warning: could not write backup-schedule cache: %v", err)
	}
}

func currentBackupScheduleState() backupScheduleState {
	backupScheduleMu.Lock()
	defer backupScheduleMu.Unlock()
	return backupScheduleLast
}

// recordScheduledAttempt stamps the scheduler's own reference point and the
// attempt time with it. The 24-hour gate reads LastScheduledAt alone, so a
// chatty restart cycle does not re-attempt inside the same window, and a backup
// taken by hand does not postpone the next automatic one. Every OTHER attempt -
// the "Back up now" button - stamps LastAttemptAt inside performBackup, so the
// dialog's status line always has a time to show.
func recordScheduledAttempt() {
	now := dbcore.NowStamp()
	backupScheduleMu.Lock()
	backupScheduleLast.LastScheduledAt = now
	backupScheduleLast.LastAttemptAt = now
	st := backupScheduleLast
	backupScheduleMu.Unlock()
	saveBackupScheduleCache(st)
}

func recordBackupOutcome(outcome, errMsg string) {
	backupScheduleMu.Lock()
	backupScheduleLast.LastOutcome = outcome
	backupScheduleLast.LastError = errMsg
	st := backupScheduleLast
	backupScheduleMu.Unlock()
	saveBackupScheduleCache(st)
}

// backupUnchangedSinceLast reports whether fp equals the fingerprint recorded
// at the last SUCCESSFUL backup (not the last attempt). Before any backup has
// ever succeeded, nothing counts as unchanged.
func backupUnchangedSinceLast(fp dbFingerprint) bool {
	backupScheduleMu.Lock()
	defer backupScheduleMu.Unlock()
	return backupScheduleLast.HaveBackupFingerprint && backupScheduleLast.LastBackupFingerprint == fp
}

// backupContentUnchangedSinceLast reports whether digest equals the one recorded
// at the last SUCCESSFUL backup. An empty digest, on either side, is never a
// match: no answer means back up.
func backupContentUnchangedSinceLast(digest string) bool {
	if digest == "" {
		return false
	}
	backupScheduleMu.Lock()
	defer backupScheduleMu.Unlock()
	return backupScheduleLast.HaveBackupFingerprint && backupScheduleLast.LastBackupDigest == digest
}

// noteFingerprintOfUnchangedContent records fp against the backup that already
// covers this content, so tomorrow's run answers from the cheap tier again
// instead of digesting every table a second time. It moves nothing else: the
// backup this state describes is still the last one actually written.
func noteFingerprintOfUnchangedContent(fp dbFingerprint) {
	backupScheduleMu.Lock()
	if !backupScheduleLast.HaveBackupFingerprint {
		backupScheduleMu.Unlock()
		return
	}
	backupScheduleLast.LastBackupFingerprint = fp
	st := backupScheduleLast
	backupScheduleMu.Unlock()
	saveBackupScheduleCache(st)
}

// startBackupScheduler launches the background goroutine: on start, and then
// every 24 hours, it attempts a backup unless one was already attempted inside
// the current window. It always starts the loop - attemptScheduledBackup
// itself is what does nothing when the feature is off, which is what lets a
// test assert "no scheduler goroutine does any work" by exercising that
// function directly rather than the sleep loop.
func startBackupScheduler() {
	loadBackupScheduleCache()
	go func() {
		for {
			if !recentlyAttemptedBackup() {
				attemptScheduledBackup()
			}
			time.Sleep(backupCheckInterval)
		}
	}()
}

func recentlyAttemptedBackup() bool {
	backupScheduleMu.Lock()
	defer backupScheduleMu.Unlock()
	if backupScheduleLast.LastScheduledAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, backupScheduleLast.LastScheduledAt)
	return err == nil && time.Since(t) < backupCheckInterval
}

// attemptScheduledBackup is the testable, no-time-dependency core of the
// scheduler - what a test drives directly instead of the sleep loop above.
func attemptScheduledBackup() {
	cfg := readBackupScheduleConfig()
	if !cfg.Enabled || strings.TrimSpace(cfg.Dir) == "" {
		// Off, or on with no destination yet: no state file is written and the
		// destination folder (if any) is never touched.
		return
	}
	recordScheduledAttempt()

	if err := validateBackupDestination(cfg.Dir); err != nil {
		log.Printf("Warning: scheduled backup skipped, destination invalid: %v", err)
		recordBackupOutcome(backupOutcomeInvalidDest, err.Error())
		return
	}

	fp := currentDBFingerprint()
	if backupUnchangedSinceLast(fp) {
		recordBackupOutcome(backupOutcomeUnchanged, "")
		return
	}
	// The file moved, which since the reader's view came into the database no
	// longer means their data did. Ask the content before spending a file.
	if digest, err := dbcore.ContentDigest(); err != nil {
		log.Printf("Warning: could not digest the database, treating it as changed: %v", err)
	} else if backupContentUnchangedSinceLast(digest) {
		noteFingerprintOfUnchangedContent(fp)
		recordBackupOutcome(backupOutcomeUnchanged, "")
		return
	}

	naming := dbcore.TimestampedBackup
	if cfg.Overwrite {
		naming = dbcore.OverwritingBackup
	}
	if _, err := performBackup(cfg.Dir, naming); err != nil {
		log.Printf("Warning: scheduled backup failed: %v", err)
	}
}

// performBackup is the ONE function that actually writes a backup file, used by
// BOTH the scheduler and the manual "Back up now" endpoint. It calls
// dbcore.BackupTo and, on success, updates and persists the same schedule state
// the scheduler reads - so a manual backup the user takes right now is
// reflected immediately in "last backup at" and suppresses a redundant
// scheduled run the next day for the same unchanged content.
//
// It stamps LastAttemptAt itself, because reaching this function IS an attempt
// however it was reached. Without that a backup taken from the button recorded
// an outcome with no time against it, and the dialog reads the time.
func performBackup(dir string, naming dbcore.BackupNaming) (string, error) {
	// Digested BEFORE the copy is taken. A write that lands while VACUUM INTO is
	// running is not in the file that was written, and a digest taken afterwards
	// would claim it was, costing the next run its backup.
	digest, derr := dbcore.ContentDigest()
	if derr != nil {
		log.Printf("Warning: could not digest the database before backing up: %v", derr)
		digest = ""
	}
	// The fingerprint is taken before the copy for the same reason: one taken
	// afterwards would record a write that landed during the copy as backed up.
	fingerprint := currentDBFingerprint()
	path, err := dbcore.BackupTo(dir, naming)
	backupScheduleMu.Lock()
	backupScheduleLast.LastAttemptAt = dbcore.NowStamp()
	if err != nil {
		backupScheduleLast.LastOutcome = backupOutcomeError
		backupScheduleLast.LastError = err.Error()
	} else {
		backupScheduleLast.LastOutcome = backupOutcomeOK
		backupScheduleLast.LastError = ""
		backupScheduleLast.LastBackupAt = dbcore.NowStamp()
		backupScheduleLast.LastBackupFingerprint = fingerprint
		backupScheduleLast.HaveBackupFingerprint = true
		backupScheduleLast.LastBackupDigest = digest
		backupScheduleLast.LastPath = path
	}
	st := backupScheduleLast
	backupScheduleMu.Unlock()
	saveBackupScheduleCache(st)
	return path, err
}

// validateBackupDestination checks that dir exists, is a directory, and is
// writable, returning a distinct error for each failure. Writability is PROVEN
// by writing a temporary file and removing it, never by inspecting permission
// bits - a bit check is wrong on a read-only mount, a full disk, or a network
// filesystem.
func validateBackupDestination(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("no destination folder is configured")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("destination folder %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("destination %s is a file, not a folder", dir)
	}
	f, err := os.CreateTemp(dir, "modelsdb-backup-writecheck-*.tmp")
	if err != nil {
		return fmt.Errorf("destination %s is not writable: %w", dir, err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return nil
}

// backupFileInfo is one file in the user's backup destination folder, as
// served to the browser and printed by the CLI.
type backupFileInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

// backupListing is the state of the user's configured destination folder.
// "none" (no folder configured), "missing" (configured but gone), "empty"
// (exists, holds no matching files), and "listed" (files present) are three
// different messages, not one blank list - each is a real, distinguishable
// situation the user needs to be told apart from the others.
type backupListing struct {
	State string           `json:"state"`
	Files []backupFileInfo `json:"files"`
}

// listConfiguredBackups reports the backup files sitting directly inside dir -
// the user's OWN destination folder, never the app's pre-migration
// <data>/.backups/. It lists every file whose name ends in ".db", with no
// recursion and no prefix filter. That is a deliberate middle ground: filtering
// on the "modelsdb-backup" prefix this app's own files carry would make a
// backup the user renamed by hand invisible (a lie about what backups exist),
// while listing literally everything in the folder would surface the user's
// unrelated files as noise. The .db extension is what every backup this app
// writes carries, and what excludes non-database clutter.
func listConfiguredBackups(dir string) backupListing {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return backupListing{State: "none", Files: []backupFileInfo{}}
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return backupListing{State: "missing", Files: []backupFileInfo{}}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return backupListing{State: "missing", Files: []backupFileInfo{}}
	}

	type found struct {
		name string
		size int64
		mod  time.Time
	}
	var files []found
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, found{name: e.Name(), size: fi.Size(), mod: fi.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })

	out := make([]backupFileInfo, 0, len(files))
	for _, f := range files {
		out = append(out, backupFileInfo{Name: f.name, Size: f.size, Modified: f.mod.UTC().Format(time.RFC3339)})
	}
	if len(out) == 0 {
		return backupListing{State: "empty", Files: out}
	}
	return backupListing{State: "listed", Files: out}
}

// printConfiguredBackups prints the second, labelled section of `modelsdb
// backups`: the user's own destination-folder backups, as opposed to the
// app's pre-migration snapshots dbcore.ListBackups already printed. It shares
// listConfiguredBackups with the /api/backup/status handler, so the folder-
// scanning logic exists in exactly one place.
func printConfiguredBackups() {
	cfg := readBackupScheduleConfig()
	dir := strings.TrimSpace(cfg.Dir)
	fmt.Println()
	if dir == "" {
		fmt.Println("Your backups in <not configured> (never pruned):")
		fmt.Println("  No destination folder is configured yet. Set one in the Info dialog's \"Files & updates\" panel to see your own backups listed here.")
		return
	}
	listing := listConfiguredBackups(dir)
	fmt.Printf("Your backups in %s (never pruned):\n", dir)
	switch listing.State {
	case "missing":
		fmt.Printf("  Configured folder %s does not currently exist.\n", dir)
	case "empty":
		fmt.Printf("  No backup files found in %s yet.\n", dir)
	case "listed":
		for _, f := range listing.Files {
			fmt.Printf("  %-36s %10d bytes  %s\n", f.Name, f.Size, f.Modified)
		}
	}
}

// ---- HTTP handlers -----------------------------------------------------

// handleBackupValidate (POST /api/backup/validate) runs validateBackupDestination
// against the body's {"dir": "..."} and reports the verdict. It persists
// NOTHING: the browser calls this on blur/change of the destination field for
// an immediate inline verdict, then separately persists the typed value via the
// existing saveFilters -> /api/settings path regardless of whether validation
// passed, so typing is never lost and a folder that starts invalid can keep
// being retried later.
func handleBackupValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Dir string `json:"dir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), decodeFailureStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := validateBackupDestination(body.Dir); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

// handleBackupRun (POST /api/backup/run) is the "Back up now" button's target:
// {"dir": "...", "overwrite": bool}. It writes only into the folder saved in
// config.jsonc: a request naming any other folder is refused, so a caller that
// reaches the port cannot make the server write a copy of the whole database
// wherever it likes. It works regardless of whether the schedule is enabled, and
// it is throttled by neither the 24h gate nor the unchanged-fingerprint skip
// (the user explicitly asked for one now) - but it DOES update the shared
// state via performBackup, so the scheduler and the modal's status line both
// see it.
func handleBackupRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Dir       string `json:"dir"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), decodeFailureStatus(err))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	saved := strings.TrimSpace(readBackupScheduleConfig().Dir)
	if saved == "" || filepath.Clean(strings.TrimSpace(body.Dir)) != filepath.Clean(saved) {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Back up now writes only into the saved destination folder. Save the folder first, then back up."})
		return
	}
	if err := validateBackupDestination(saved); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	naming := dbcore.TimestampedBackup
	if body.Overwrite {
		naming = dbcore.OverwritingBackup
	}
	path, err := performBackup(saved, naming)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	var size int64
	if info, serr := os.Stat(path); serr == nil {
		size = info.Size()
	}
	log.Printf("ok /api/backup/run: wrote %s (%d bytes)", path, size)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "path": path, "bytes": size})
}

// handleBackupStatus (GET /api/backup/status) is one combined read for the
// modal to fetch on open, mirroring how showPaths() does one /api/paths fetch:
// the saved config, the scheduler's own state, and the destination folder's
// listing, all in one response.
func handleBackupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := readBackupScheduleConfig()
	st := currentBackupScheduleState()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled":           cfg.Enabled,
		"dir":               cfg.Dir,
		"overwrite":         cfg.Overwrite,
		"last_backup_at":    st.LastBackupAt,
		"last_attempt_at":   st.LastAttemptAt,
		"last_scheduled_at": st.LastScheduledAt,
		"last_outcome":      st.LastOutcome,
		"last_error":        st.LastError,
		"listing":           listConfiguredBackups(cfg.Dir),
	})
}

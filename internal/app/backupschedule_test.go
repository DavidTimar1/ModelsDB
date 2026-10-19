package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"modelsdb/internal/paths"
	"modelsdb/internal/store"
)

// resetBackupScheduleState clears the in-memory scheduler state between tests,
// since it lives in package-level vars.
func resetBackupScheduleState(t *testing.T) {
	t.Helper()
	backupScheduleMu.Lock()
	backupScheduleLast = backupScheduleState{}
	backupScheduleMu.Unlock()
}

// setBackupSettings puts the daily backup's configuration where the scheduler
// reads it, through the same call the Info dialog's save makes. The map keys are
// the ones the page sends, so a test reads as the blob the browser POSTs.
func setBackupSettings(t *testing.T, m map[string]interface{}) {
	t.Helper()
	enabled, _ := m["backup_enabled"].(bool)
	dir, _ := m["backup_dir"].(string)
	overwrite, _ := m["backup_overwrite"].(bool)
	if err := paths.SaveBackupSettings(enabled, dir, overwrite); err != nil {
		t.Fatal(err)
	}
}

// backupScheduleTestSetup gives a test a hermetic migrated DB (via migratedDB)
// plus its own config and cache dir (config.jsonc + the schedule cache file),
// and resets both the in-memory scheduler state and the backup configuration.
func backupScheduleTestSetup(t *testing.T) {
	t.Helper()
	migratedDB(t)
	resetBackupScheduleState(t)
	cfgDir := t.TempDir()
	paths.ConfigDir = cfgDir
	paths.ConfigFile = filepath.Join(cfgDir, "config.jsonc")
	paths.CacheDir = cfgDir
	paths.BackupScheduleCacheFile = filepath.Join(cfgDir, "backup-schedule.json")
	if err := paths.SaveBackupSettings(false, "", false); err != nil {
		t.Fatal(err)
	}
}

func importHFModel(t *testing.T, name, slug string) {
	t.Helper()
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": name, "source": "huggingface", "hf_slug": slug,
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
}

// 1. Scheduler off (setting absent) -> attemptScheduledBackup writes no file
// and writes no schedule-state file either.
func TestAttemptScheduledBackupDoesNothingWhenOff(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	setBackupSettings(t, map[string]interface{}{"backup_dir": dest})

	attemptScheduledBackup()

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("destination folder should be untouched when the feature is off, found %d entries", len(entries))
	}
	if _, err := os.Stat(paths.BackupScheduleCacheFile); !os.IsNotExist(err) {
		t.Fatalf("no schedule-state file should be written when the feature is off (stat err = %v)", err)
	}
}

// 2. Three separate unit tests on validateBackupDestination, each producing a
// distinct, distinguishable verdict.
func TestValidateBackupDestinationNonexistentPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	if err := validateBackupDestination(dir); err == nil {
		t.Fatal("expected an error for a nonexistent path")
	}
}

func TestValidateBackupDestinationFileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-folder.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateBackupDestination(file); err == nil {
		t.Fatal("expected an error for a path that is a file, not a folder")
	}
}

func TestValidateBackupDestinationNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not block writes")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "readonly")
	if err := os.Mkdir(sub, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sub, 0700) }) // let t.TempDir() clean up
	if err := validateBackupDestination(sub); err == nil {
		t.Fatal("expected an error for a non-writable folder")
	}
}

func TestValidateBackupDestinationThreeMessagesAreDistinct(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits do not block writes")
	}
	dir := t.TempDir()
	nonexistent := filepath.Join(dir, "gone")
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	notWritable := filepath.Join(dir, "locked")
	if err := os.Mkdir(notWritable, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(notWritable, 0700) })

	e1 := validateBackupDestination(nonexistent)
	e2 := validateBackupDestination(file)
	e3 := validateBackupDestination(notWritable)
	if e1 == nil || e2 == nil || e3 == nil {
		t.Fatalf("all three should fail: %v / %v / %v", e1, e2, e3)
	}
	if e1.Error() == e2.Error() || e1.Error() == e3.Error() || e2.Error() == e3.Error() {
		t.Fatalf("the three verdicts must be distinct, got %q / %q / %q", e1, e2, e3)
	}
}

// 3. A stale recorded LastScheduledAt -> the scheduler's decision function says
// "run now"; a fresh one says "don't".
func TestRecentlyAttemptedBackup(t *testing.T) {
	backupScheduleTestSetup(t)

	if recentlyAttemptedBackup() {
		t.Fatal("with no recorded attempt, recentlyAttemptedBackup should be false")
	}

	backupScheduleMu.Lock()
	backupScheduleLast.LastScheduledAt = time.Now().UTC().Add(-25 * time.Hour).Format(time.RFC3339)
	backupScheduleMu.Unlock()
	if recentlyAttemptedBackup() {
		t.Fatal("a stale (>24h) attempt should say run now (recentlyAttemptedBackup = false)")
	}

	backupScheduleMu.Lock()
	backupScheduleLast.LastScheduledAt = time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	backupScheduleMu.Unlock()
	if !recentlyAttemptedBackup() {
		t.Fatal("a fresh (<24h) attempt should say don't run (recentlyAttemptedBackup = true)")
	}
}

// 3b. A backup taken from the button records an attempt, like a scheduled one,
// so the dialog's status line has a time to show instead of "Never attempted
// yet." - and it leaves the SCHEDULER's own reference point alone, so it cannot
// postpone the next automatic copy.
func TestManualBackupRecordsAnAttemptAndLeavesTheScheduleAlone(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	setBackupSettings(t, map[string]interface{}{"backup_dir": dest})

	body := strings.NewReader(`{"dir":` + strconv.Quote(dest) + `,"overwrite":false}`)
	rec := httptest.NewRecorder()
	handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/api/backup/run", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/backup/run status = %d", rec.Code)
	}

	st := currentBackupScheduleState()
	if st.LastAttemptAt == "" {
		t.Fatal("a backup taken by hand recorded no attempt, so the dialog reads \"Never attempted yet.\"")
	}
	if st.LastOutcome != backupOutcomeOK {
		t.Fatalf("outcome = %q, want %q", st.LastOutcome, backupOutcomeOK)
	}
	if st.LastScheduledAt != "" {
		t.Fatal("a manual backup moved the scheduler's own reference point, which would postpone the next automatic copy")
	}
	if recentlyAttemptedBackup() {
		t.Fatal("a manual backup made the scheduler skip its next run")
	}

	// What the dialog actually reads back.
	rec = httptest.NewRecorder()
	handleBackupStatus(rec, httptest.NewRequest(http.MethodGet, "/api/backup/status", nil))
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["last_attempt_at"] == "" {
		t.Fatal("/api/backup/status returned an empty last_attempt_at after a manual backup")
	}
}

// 4. Two calls to attemptScheduledBackup() with no intervening DB write ->
// exactly one file in the destination.
func TestAttemptScheduledBackupSkipsWhenUnchanged(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	setBackupSettings(t, map[string]interface{}{"backup_enabled": true, "backup_dir": dest})

	attemptScheduledBackup()
	attemptScheduledBackup()

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("two runs with no DB write should leave exactly one file, found %d", len(entries))
	}
	st := currentBackupScheduleState()
	if st.LastOutcome != backupOutcomeUnchanged {
		t.Fatalf("second run's outcome = %q, want %q", st.LastOutcome, backupOutcomeUnchanged)
	}
}

// 5. A real store.SetCustomValue write between two calls (models table
// untouched) -> the SECOND call still fires. This is the case a naive
// models.updated_at-based detector would miss, because SetCustomValue writes
// only to custom_values and never touches the models table - the file-level
// fingerprint catches it anyway because ANY committed write moves the WAL's
// (size, mtime), regardless of which table it touched.
func TestAttemptScheduledBackupFiresOnCustomColumnEditAlone(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	setBackupSettings(t, map[string]interface{}{"backup_enabled": true, "backup_dir": dest})

	importHFModel(t, "M", "x/m")
	colID, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold"})
	if err != nil {
		t.Fatal(err)
	}

	attemptScheduledBackup()

	// A write that touches ONLY custom_values, never models.
	if err := setOwnValue("M", colID, "gold"); err != nil {
		t.Fatal(err)
	}
	// dbcore's timestamped filename has SECOND granularity (fsStamp), so two
	// runs inside the same wall-clock second would collide on the same name
	// and the second rename would silently replace the first - a real
	// constraint of the shared backup engine, not of this test. Cross a
	// second boundary so the two runs are independently visible.
	time.Sleep(1100 * time.Millisecond)

	attemptScheduledBackup()

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("a custom-column-only edit between runs should still produce a second backup, found %d files", len(entries))
	}
}

// 6. Overwrite mode ticked -> repeated runs replace one file, name never
// changes; unticked -> each run adds a new timestamped file.
func TestAttemptScheduledBackupOverwriteMode(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	setBackupSettings(t, map[string]interface{}{"backup_enabled": true, "backup_dir": dest, "backup_overwrite": true})

	importHFModel(t, "M1", "x/m1")
	attemptScheduledBackup()

	importHFModel(t, "M2", "x/m2")
	attemptScheduledBackup()

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("overwrite mode should leave exactly one file across repeated changed runs, found %d", len(entries))
	}
	if entries[0].Name() != "modelsdb-backup.db" {
		t.Fatalf("overwrite mode's filename should never change, got %q", entries[0].Name())
	}
}

func TestAttemptScheduledBackupTimestampedMode(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	setBackupSettings(t, map[string]interface{}{"backup_enabled": true, "backup_dir": dest})

	importHFModel(t, "M1", "x/m1")
	attemptScheduledBackup()

	importHFModel(t, "M2", "x/m2")
	// dbcore's timestamped filename has SECOND granularity (fsStamp), so two
	// runs inside the same wall-clock second would collide on the same name -
	// a real constraint of the shared backup engine, not of this test.
	time.Sleep(1100 * time.Millisecond)
	attemptScheduledBackup()

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("timestamped mode should add one new file per changed run, found %d", len(entries))
	}
}

// 7. Seed the destination folder with unrelated files first -> they all
// survive several scheduler runs untouched (B8).
func TestAttemptScheduledBackupNeverTouchesUnrelatedFiles(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	unrelated := filepath.Join(dest, "my-photo.jpg")
	if err := os.WriteFile(unrelated, []byte("not a backup"), 0644); err != nil {
		t.Fatal(err)
	}
	setBackupSettings(t, map[string]interface{}{"backup_enabled": true, "backup_dir": dest})

	names := []string{"Model A", "Model B", "Model C"}
	slugs := []string{"x/a", "x/b", "x/c"}
	for i := range names {
		importHFModel(t, names[i], slugs[i])
		attemptScheduledBackup()
	}

	b, err := os.ReadFile(unrelated)
	if err != nil {
		t.Fatalf("the unrelated file must survive: %v", err)
	}
	if string(b) != "not a backup" {
		t.Fatalf("the unrelated file's content changed: %q", b)
	}
}

// 8. Remove the destination folder mid-life -> a logged error, LastOutcome =
// "invalid destination", server/loop keeps working, and it recovers (writes
// normally) once the folder is restored on a later attempt.
func TestAttemptScheduledBackupRecoversAfterDestinationReturns(t *testing.T) {
	backupScheduleTestSetup(t)
	parent := t.TempDir()
	dest := filepath.Join(parent, "gone-for-a-while")
	if err := os.Mkdir(dest, 0755); err != nil {
		t.Fatal(err)
	}
	setBackupSettings(t, map[string]interface{}{"backup_enabled": true, "backup_dir": dest})

	if err := os.RemoveAll(dest); err != nil {
		t.Fatal(err)
	}

	attemptScheduledBackup()
	st := currentBackupScheduleState()
	if st.LastOutcome != backupOutcomeInvalidDest {
		t.Fatalf("outcome with a missing destination = %q, want %q", st.LastOutcome, backupOutcomeInvalidDest)
	}
	if st.LastError == "" {
		t.Fatal("expected a recorded error message")
	}

	if err := os.Mkdir(dest, 0755); err != nil {
		t.Fatal(err)
	}
	attemptScheduledBackup()
	st = currentBackupScheduleState()
	if st.LastOutcome != backupOutcomeOK {
		t.Fatalf("outcome once the destination is restored = %q, want %q", st.LastOutcome, backupOutcomeOK)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one backup file after recovery, found %d", len(entries))
	}
}

// 9. Write schedule state, then POST an ordinary filter-bar settings blob to
// handleSettings, then read the schedule state back unchanged. This proves the
// scheduler's state truly lives in its own cache file, out of reach of anything
// the page saves.
func TestBackupScheduleStateSurvivesAFilterBarSettingsWrite(t *testing.T) {
	backupScheduleTestSetup(t)
	recordScheduledAttempt()
	recordBackupOutcome(backupOutcomeOK, "")
	before := currentBackupScheduleState()

	// An ordinary filter-bar POST /api/settings, exactly the shape saveFilters()
	// builds - it carries none of the scheduler's own state.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"set":{"f_search":"gpt","auto_update":true},"delete":[]}`))
	handleSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/settings status = %d", rec.Code)
	}

	// Reload from disk to prove it is the persisted state, not just the
	// in-memory copy, that survived.
	resetBackupScheduleState(t)
	loadBackupScheduleCache()
	after := currentBackupScheduleState()

	if after.LastAttemptAt != before.LastAttemptAt || after.LastOutcome != before.LastOutcome {
		t.Fatalf("schedule state changed after a filter-bar settings write: before=%+v after=%+v", before, after)
	}
}

// 10. /api/backup/status's listing block: the four states.
func TestListConfiguredBackupsFourStates(t *testing.T) {
	if got := listConfiguredBackups("").State; got != "none" {
		t.Errorf("empty dir -> state %q, want \"none\"", got)
	}
	if got := listConfiguredBackups(filepath.Join(t.TempDir(), "nope")).State; got != "missing" {
		t.Errorf("nonexistent dir -> state %q, want \"missing\"", got)
	}
	empty := t.TempDir()
	if got := listConfiguredBackups(empty).State; got != "empty" {
		t.Errorf("empty existing dir -> state %q, want \"empty\"", got)
	}
	populated := t.TempDir()
	if err := os.WriteFile(filepath.Join(populated, "modelsdb-backup-20260101-000000.db"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// A non-.db file must not appear in the listing.
	if err := os.WriteFile(filepath.Join(populated, "notes.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	listing := listConfiguredBackups(populated)
	if listing.State != "listed" {
		t.Errorf("populated dir -> state %q, want \"listed\"", listing.State)
	}
	if len(listing.Files) != 1 || listing.Files[0].Name != "modelsdb-backup-20260101-000000.db" {
		t.Errorf("listing = %+v, want exactly the one .db file", listing.Files)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// everything it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

// 11. `modelsdb backups`' two-section output: with and without a configured
// destination folder, asserting both labelled sections appear and the right
// one of the three messages shows.
func TestPrintConfiguredBackupsWithNoDestination(t *testing.T) {
	backupScheduleTestSetup(t)
	out := captureStdout(t, printConfiguredBackups)
	if !strings.Contains(out, "Your backups in <not configured>") {
		t.Errorf("expected the no-destination message, got: %s", out)
	}
}

func TestPrintConfiguredBackupsWithDestination(t *testing.T) {
	backupScheduleTestSetup(t)
	dest := t.TempDir()
	setBackupSettings(t, map[string]interface{}{"backup_dir": dest})
	out := captureStdout(t, printConfiguredBackups)
	if !strings.Contains(out, dest) {
		t.Errorf("expected the configured folder to be named, got: %s", out)
	}
	if !strings.Contains(out, "No backup files found") {
		t.Errorf("expected the empty-folder message, got: %s", out)
	}
}

// Back up now writes only into the saved destination folder: a request naming
// another folder, or any folder while none is saved, writes nothing.
func TestBackupRunRefusesAFolderThatIsNotTheSavedOne(t *testing.T) {
	backupScheduleTestSetup(t)
	saved := t.TempDir()
	other := t.TempDir()

	run := func(dir string) map[string]interface{} {
		t.Helper()
		body := strings.NewReader(`{"dir":` + strconv.Quote(dir) + `,"overwrite":false}`)
		rec := httptest.NewRecorder()
		handleBackupRun(rec, httptest.NewRequest(http.MethodPost, "/api/backup/run", body))
		var got map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	entries := func(dir string) int {
		t.Helper()
		e, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		return len(e)
	}

	if got := run(other); got["ok"] != false {
		t.Fatalf("with no folder saved, a named folder was accepted: %v", got)
	}
	setBackupSettings(t, map[string]interface{}{"backup_dir": saved})
	if got := run(other); got["ok"] != false {
		t.Fatalf("a folder other than the saved one was accepted: %v", got)
	}
	if entries(other) != 0 {
		t.Fatal("a backup was written into a folder that is not the saved one")
	}
	if got := run(saved + string(filepath.Separator)); got["ok"] != true {
		t.Fatalf("the saved folder (with a trailing separator) was refused: %v", got)
	}
	if entries(saved) != 1 {
		t.Fatalf("saved folder holds %d files, want 1", entries(saved))
	}
}

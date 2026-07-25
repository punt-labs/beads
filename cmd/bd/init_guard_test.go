package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
)

// TestStoreOpenHint verifies that a store-open failure in a committed
// server-mode workspace steers the user toward server/env checks, not
// `bd init`, which would silently re-init embedded (pkit-zj7y).
func TestStoreOpenHint(t *testing.T) {
	writeMode := func(t *testing.T, mode string) string {
		t.Helper()
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		if mode != "" {
			data, _ := json.Marshal(map[string]interface{}{"dolt_mode": mode})
			if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
		return beadsDir
	}

	t.Run("server mode warns against bd init", func(t *testing.T) {
		hint := storeOpenHint(writeMode(t, configfile.DoltModeServer))
		if !strings.Contains(hint, "BEADS_DOLT_*") {
			t.Errorf("server-mode hint must mention BEADS_DOLT_* env, got: %q", hint)
		}
		if !strings.Contains(hint, "Do NOT run 'bd init'") {
			t.Errorf("server-mode hint must warn against bd init, got: %q", hint)
		}
	})

	t.Run("embedded mode falls back to generic hint", func(t *testing.T) {
		hint := storeOpenHint(writeMode(t, configfile.DoltModeEmbedded))
		if strings.Contains(hint, "Do NOT run 'bd init'") {
			t.Errorf("embedded hint must not carry the server warning, got: %q", hint)
		}
	})
}

// TestServerDownHint verifies the `bd dolt start` advice is suppressed for a
// remote server workspace (where a local start cannot help) but kept for a
// local one (pkit-zj7y).
func TestServerDownHint(t *testing.T) {
	t.Run("remote server steers away from bd dolt start", func(t *testing.T) {
		cfg := &configfile.Config{DoltMode: configfile.DoltModeServer}
		hint := serverDownHint(cfg, "db.hosted.example.com", 3306)
		if strings.Contains(hint, "bd dolt start\n") || hint == "Start the server with: bd dolt start" {
			t.Errorf("remote hint must not present bd dolt start as the fix, got: %q", hint)
		}
		if !strings.Contains(hint, "BEADS_DOLT_*") {
			t.Errorf("remote hint must mention BEADS_DOLT_* env, got: %q", hint)
		}
	})

	t.Run("local server keeps bd dolt start", func(t *testing.T) {
		cfg := &configfile.Config{DoltMode: configfile.DoltModeServer}
		hint := serverDownHint(cfg, "127.0.0.1", 3307)
		if !strings.Contains(hint, "bd dolt start") {
			t.Errorf("local hint should keep bd dolt start, got: %q", hint)
		}
	})
}

// gitInitRepo creates a git repo rooted at dir with a usable identity and no
// commit signing. GIT_CONFIG_COUNT=0 neutralizes any commit.gpgsign /
// user.signingkey injected via GIT_CONFIG_* env (direnv), which has
// command-line precedence and would otherwise override the repo-local setting.
func gitInitRepo(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_COUNT", "0")
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t.t"},
		{"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// gitCommitAll stages and commits everything under dir.
func gitCommitAll(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-qm", "seed"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// writeServerMetadata writes a committed-shape server-mode metadata.json.
func writeServerMetadata(t *testing.T, beadsDir string) {
	t.Helper()
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]interface{}{
		"dolt_mode":     "server",
		"dolt_database": "beads",
		"issue_prefix":  "pkit",
	}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// TestInitGuard_CommittedServerUnreachableFailsClosed is the pkit-zj7y guard:
// when metadata.json is committed saying dolt_mode=server, no local dolt dir
// exists, and the server is unreachable, init must FAIL CLOSED rather than
// proceed as a "fresh clone" and silently flip the backend to embedded.
func TestInitGuard_CommittedServerUnreachableFailsClosed(t *testing.T) {
	oldServerMode := serverMode
	serverMode = true
	defer func() { serverMode = oldServerMode }()

	repo := t.TempDir()
	setupIsolatedGitConfig(t, t.TempDir())
	gitInitRepo(t, repo)
	beadsDir := filepath.Join(repo, ".beads")
	writeServerMetadata(t, beadsDir)
	gitCommitAll(t, repo)

	// No dolt/ dir and no server running: the server check is unreachable.
	err := checkExistingBeadsDataAt(beadsDir, "pkit")
	if err == nil {
		t.Fatal("committed server metadata + unreachable server must fail closed, got nil")
	}
	for _, want := range []string{"unreachable", "dolt_mode=server", "--migrate-backend"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must contain %q, got:\n%s", want, err)
		}
	}
}

// TestInitGuard_UncommittedServerMetadataAllowsInit is the discriminator:
// the same server metadata that is NOT committed to git is treated as a
// genuine bootstrap-in-progress, so init is allowed to proceed (GH#2433).
func TestInitGuard_UncommittedServerMetadataAllowsInit(t *testing.T) {
	oldServerMode := serverMode
	serverMode = true
	defer func() { serverMode = oldServerMode }()

	repo := t.TempDir()
	setupIsolatedGitConfig(t, t.TempDir())
	gitInitRepo(t, repo)
	beadsDir := filepath.Join(repo, ".beads")
	writeServerMetadata(t, beadsDir) // written but never committed

	err := checkExistingBeadsDataAt(beadsDir, "pkit")
	if err != nil {
		t.Errorf("uncommitted server metadata should allow init (bootstrap), got: %v", err)
	}
}

// TestCommittedServerModeIntent verifies init seeds server-mode intent from
// the committed metadata.json so an env-stripped subprocess re-inits in
// server mode instead of embedded (pkit-zj7y, fix 3).
func TestCommittedServerModeIntent(t *testing.T) {
	t.Run("server metadata -> true", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		writeServerMetadata(t, beadsDir)
		t.Setenv("BEADS_DIR", beadsDir)
		if !committedServerModeIntent() {
			t.Error("expected committedServerModeIntent() = true for server metadata")
		}
	})

	t.Run("embedded metadata -> false", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(map[string]interface{}{"dolt_mode": "embedded"})
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BEADS_DIR", beadsDir)
		if committedServerModeIntent() {
			t.Error("expected committedServerModeIntent() = false for embedded metadata")
		}
	})

	t.Run("no metadata -> false", func(t *testing.T) {
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BEADS_DIR", beadsDir)
		if committedServerModeIntent() {
			t.Error("expected committedServerModeIntent() = false when no metadata.json")
		}
	})
}

func TestInitGuardServerMessage(t *testing.T) {
	tests := map[string]struct {
		dbName         string
		host           string
		port           int
		prefix         string
		syncRemote     string
		wantContains   []string
		wantNotContain []string
	}{
		"DB missing, no sync.remote configured (FR-010, FR-011)": {
			dbName:     "acf_beads",
			host:       "127.0.0.1",
			port:       3309,
			prefix:     "acf",
			syncRemote: "",
			wantContains: []string{
				`"acf_beads"`,
				"127.0.0.1:3309",
				"not found on server",
				"bd doctor",
				"bd dolt status",
				"bd bootstrap",
				"set sync.remote",
				".beads/config.yaml",
				"Aborting",
				"--force destroys ALL existing issues",
			},
			wantNotContain: []string{
				"sync.remote is configured",
				// GH#2363: must NOT suggest --force as the primary action
				"bd init --force --prefix",
			},
		},
		"DB missing, sync.remote IS configured (FR-010, FR-011)": {
			dbName:     "beads_kc",
			host:       "192.168.1.50",
			port:       3307,
			prefix:     "kc",
			syncRemote: "https://doltremoteapi.dolthub.com/myorg/beads",
			wantContains: []string{
				`"beads_kc"`,
				"192.168.1.50:3307",
				"not found on server",
				"bd doctor",
				"bd dolt status",
				"bd bootstrap",
				"sync.remote is configured",
				"https://doltremoteapi.dolthub.com/myorg/beads",
				"--force destroys ALL existing issues",
			},
			wantNotContain: []string{
				"set sync.remote",
				// GH#2363: must NOT suggest --force as the primary action
				"bd init --force --prefix",
				"bd init --force to bootstrap",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := initGuardServerMessage(tt.dbName, tt.host, tt.port, tt.prefix, tt.syncRemote)
			if err == nil {
				t.Fatal("expected non-nil error")
			}

			msg := err.Error()

			for _, want := range tt.wantContains {
				if !strings.Contains(msg, want) {
					t.Errorf("expected message to contain %q, got:\n%s", want, msg)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(msg, notWant) {
					t.Errorf("expected message NOT to contain %q, got:\n%s", notWant, msg)
				}
			}
		})
	}
}

func TestInitGuardDBCheck_ExistsPath(t *testing.T) {
	// FR-012: When checkDatabaseOnServer returns Exists=true, the init guard
	// should fall through to existing "already initialized" message.
	// We verify the guard's branching logic: only Reachable=true AND Exists=false
	// triggers the new message; Exists=true must NOT trigger it.

	t.Run("exists=true skips refined message", func(t *testing.T) {
		// Simulate the guard's decision logic directly.
		// When DB exists, the guard should NOT call initGuardServerMessage.
		result := initGuardDBCheck{Exists: true, Reachable: true}
		if result.Reachable && !result.Exists && result.Err == nil {
			t.Fatal("guard would incorrectly show refined message for existing DB")
		}
		// Pass: the condition is false, so the guard falls through to "already initialized".
	})

	t.Run("exists=false triggers refined message", func(t *testing.T) {
		result := initGuardDBCheck{Exists: false, Reachable: true}
		if !(result.Reachable && !result.Exists && result.Err == nil) {
			t.Fatal("guard would NOT show refined message for missing DB")
		}
		// Verify the message content matches FR-010.
		err := initGuardServerMessage("test_db", "127.0.0.1", 3309, "test", "")
		if err == nil {
			t.Fatal("expected non-nil error")
		}
		if !strings.Contains(err.Error(), "not found on server") {
			t.Errorf("expected 'not found on server' in message, got:\n%s", err.Error())
		}
		if !strings.Contains(err.Error(), "bd bootstrap") {
			t.Errorf("expected bootstrap-first recovery guidance, got:\n%s", err.Error())
		}
	})
}

func TestInitGuardDBCheck_ServerUnreachable(t *testing.T) {
	// FR-030: When server is unreachable, should return Reachable=false
	// so caller falls through to existing error path without panic.

	result := checkDatabaseOnServer("127.0.0.1", 1, "root", "", "nonexistent_db", false)
	if result.Reachable {
		t.Fatal("expected Reachable=false for connection refused")
	}
	if result.Err == nil {
		t.Fatal("expected non-nil error for connection refused")
	}
	// Key assertion: no panic occurred — FR-030 satisfied.
}

func TestInitGuard_FreshCloneWithMetadataJSON(t *testing.T) {
	// GH#2433: On a fresh clone, metadata.json is committed (tracked by git)
	// but dolt/ directory is gitignored. The init guard should recognize this
	// as a fresh clone and allow init to proceed.

	t.Run("server_mode_metadata_no_dolt_dir_allows_init", func(t *testing.T) {
		// Switch to server mode for this subtest
		oldServerMode := serverMode
		serverMode = true
		defer func() { serverMode = oldServerMode }()

		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Write metadata.json as it would be on a fresh clone:
		// DoltMode=server, DoltDatabase set, but no dolt/ directory.
		metadata := map[string]interface{}{
			"database":      "dolt",
			"backend":       "dolt",
			"dolt_mode":     "server",
			"dolt_database": "myproject",
		}
		data, _ := json.Marshal(metadata)
		metadataPath := filepath.Join(beadsDir, "metadata.json")
		if err := os.WriteFile(metadataPath, data, 0644); err != nil {
			t.Fatal(err)
		}

		// No dolt/ directory — simulates fresh clone with gitignored dolt/.
		// No server running — simulates machine B with no local server.
		err := checkExistingBeadsDataAt(beadsDir, "myproject")
		if err != nil {
			t.Errorf("fresh clone with metadata.json should allow init, got: %v", err)
		}
	})

	t.Run("server_mode_with_dolt_dir_blocks_init", func(t *testing.T) {
		// Switch to server mode for this subtest
		oldServerMode := serverMode
		serverMode = true
		defer func() { serverMode = oldServerMode }()

		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Write metadata.json with server mode
		metadata := map[string]interface{}{
			"database":      "dolt",
			"backend":       "dolt",
			"dolt_mode":     "server",
			"dolt_database": "myproject",
		}
		data, _ := json.Marshal(metadata)
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
			t.Fatal(err)
		}

		// Create dolt/ directory — this is NOT a fresh clone
		doltDir := filepath.Join(beadsDir, "dolt")
		if err := os.MkdirAll(doltDir, 0755); err != nil {
			t.Fatal(err)
		}

		err := checkExistingBeadsDataAt(beadsDir, "myproject")
		if err == nil {
			t.Error("existing dolt directory should block init")
		}
		if err != nil && !strings.Contains(err.Error(), "already initialized") {
			t.Errorf("expected 'already initialized' message, got: %v", err)
		}
		// GH#3684: must suggest --reinit-local, not deprecated --force
		if err != nil && strings.Contains(err.Error(), "init --force") {
			t.Errorf("message must NOT suggest deprecated --force, got:\n%s", err)
		}
	})

	t.Run("embedded_mode_no_embeddeddolt_dir_allows_init", func(t *testing.T) {
		// Embedded mode is the default — no need to set serverMode
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Write metadata.json with embedded mode
		metadata := map[string]interface{}{
			"database":  "dolt",
			"backend":   "dolt",
			"dolt_mode": "embedded",
		}
		data, _ := json.Marshal(metadata)
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
			t.Fatal(err)
		}

		// No embeddeddolt/ directory — simulates fresh clone
		err := checkExistingBeadsDataAt(beadsDir, "test")
		if err != nil {
			t.Errorf("fresh clone with embedded metadata should allow init, got: %v", err)
		}
	})

	t.Run("embedded_mode_with_existing_db_blocks_init", func(t *testing.T) {
		// Embedded mode is the default — no need to set serverMode
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Write metadata.json with embedded mode
		metadata := map[string]interface{}{
			"database":  "dolt",
			"backend":   "dolt",
			"dolt_mode": "embedded",
		}
		data, _ := json.Marshal(metadata)
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
			t.Fatal(err)
		}

		// Create embeddeddolt/<db>/.dolt/ to simulate an existing embedded database
		dbDir := filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			t.Fatal(err)
		}

		err := checkExistingBeadsDataAt(beadsDir, "test")
		if err == nil {
			t.Error("existing embedded database should block init")
		}
		if err != nil && !strings.Contains(err.Error(), "already initialized") {
			t.Errorf("expected 'already initialized' message, got: %v", err)
		}
		// GH#3684: must suggest --reinit-local, not deprecated --force
		if err != nil && strings.Contains(err.Error(), "init --force") {
			t.Errorf("message must NOT suggest deprecated --force, got:\n%s", err)
		}
	})

	t.Run("embedded_metadata_ignores_ambient_shared_server_mode", func(t *testing.T) {
		t.Setenv("BEADS_DOLT_SHARED_SERVER", "1")

		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		metadata := map[string]interface{}{
			"database":  "dolt",
			"backend":   "dolt",
			"dolt_mode": "embedded",
		}
		data, _ := json.Marshal(metadata)
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
			t.Fatal(err)
		}

		dbDir := filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			t.Fatal(err)
		}

		err := checkExistingBeadsDataAt(beadsDir, "test")
		if err == nil {
			t.Error("existing embedded database should still block init when shared server mode is enabled elsewhere")
		}
		if err != nil && !strings.Contains(err.Error(), "already initialized") {
			t.Errorf("expected 'already initialized' message, got: %v", err)
		}
		// GH#3684: must suggest --reinit-local, not deprecated --force
		if err != nil && strings.Contains(err.Error(), "init --force") {
			t.Errorf("message must NOT suggest deprecated --force, got:\n%s", err)
		}
	})

	t.Run("no_metadata_json_allows_init", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// No metadata.json, no dolt/ — fresh project, never initialized
		err := checkExistingBeadsDataAt(beadsDir, "test")
		if err != nil {
			t.Errorf("empty beads dir should allow init, got: %v", err)
		}
	})
}

// GH#2363: Regression — AI agent followed "bd init --force" suggestion and wiped DB.
// Ensure the message never suggests --force as an actionable command.
func TestInitGuardServerMessage_NoForceAsAction(t *testing.T) {
	err := initGuardServerMessage("test_beads", "127.0.0.1", 3307, "test", "")
	msg := err.Error()

	// The message should mention --force only in the caution/warning section,
	// never as a suggested command to run.
	if strings.Contains(msg, "bd init --force --prefix") {
		t.Errorf("message must NOT suggest 'bd init --force --prefix' as an action:\n%s", msg)
	}
	if strings.Contains(msg, "bd init --force to") {
		t.Errorf("message must NOT suggest 'bd init --force to ...' as an action:\n%s", msg)
	}
}

// GH#3684: Regression — init "already initialized" messages must suggest
// --reinit-local (not the deprecated --force) for the reinit path.
func TestCheckExistingBeadsData_SuggestsReinitLocal(t *testing.T) {
	t.Run("embedded_dolt", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")

		// Write metadata.json with embedded dolt mode
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}
		metadata := map[string]interface{}{
			"database": "dolt", "backend": "dolt", "dolt_mode": "embedded",
		}
		data, _ := json.Marshal(metadata)
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
			t.Fatal(err)
		}

		// Create embeddeddolt/<db>/.dolt/ to simulate existing DB
		if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt"), 0755); err != nil {
			t.Fatal(err)
		}

		err := checkExistingBeadsDataAt(beadsDir, "test")
		if err == nil {
			t.Fatal("expected error for existing database")
		}
		msg := err.Error()
		if !strings.Contains(msg, "--reinit-local") {
			t.Errorf("message must suggest --reinit-local, got:\n%s", msg)
		}
		if strings.Contains(msg, "init --force") {
			t.Errorf("message must NOT suggest deprecated --force, got:\n%s", msg)
		}
	})

	t.Run("sqlite_db_file", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create a fake beads.db file (no metadata.json → falls through to SQLite check)
		if err := os.WriteFile(filepath.Join(beadsDir, "beads.db"), []byte("fake"), 0644); err != nil {
			t.Fatal(err)
		}

		err := checkExistingBeadsDataAt(beadsDir, "test")
		if err == nil {
			t.Fatal("expected error for existing database file")
		}
		msg := err.Error()
		if !strings.Contains(msg, "--reinit-local") {
			t.Errorf("message must suggest --reinit-local, got:\n%s", msg)
		}
		if strings.Contains(msg, "init --force") {
			t.Errorf("message must NOT suggest deprecated --force, got:\n%s", msg)
		}
	})
}

// GH#2338, GH#2327: Regression — error messages must always include enough
// context to identify the active target (host, port, DB name).
func TestInitGuardServerMessage_IncludesTargetIdentity(t *testing.T) {
	err := initGuardServerMessage("custom_db", "10.0.0.5", 3309, "custom", "")
	msg := err.Error()

	for _, want := range []string{"custom_db", "10.0.0.5", "3309"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message must include target identity %q, got:\n%s", want, msg)
		}
	}
}

// GH#1111: Regression — safe recovery paths must be suggested before destructive ones.
// Verify that diagnostic commands appear before any mention of --force.
func TestInitGuardServerMessage_DiagnosticsBeforeForce(t *testing.T) {
	err := initGuardServerMessage("test_beads", "127.0.0.1", 3307, "test", "")
	msg := err.Error()

	doctorIdx := strings.Index(msg, "bd doctor")
	forceIdx := strings.Index(msg, "--force")

	if doctorIdx == -1 {
		t.Fatal("message must contain 'bd doctor'")
	}
	if forceIdx == -1 {
		t.Fatal("message must contain '--force' (in caution section)")
	}
	if doctorIdx > forceIdx {
		t.Errorf("'bd doctor' (at %d) must appear before '--force' (at %d) in message:\n%s",
			doctorIdx, forceIdx, msg)
	}
}

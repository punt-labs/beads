package main

import (
	"fmt"
	"strings"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/configfile"
)

// ensureDirectMode makes sure the CLI is operating in direct-storage mode.
func ensureDirectMode(_ string) error {
	return ensureStoreActive()
}

// storeOpenHint returns guidance for a failed store open at beadsDir. For a
// workspace committed to server mode it steers the user toward server/env
// checks rather than `bd init`, which would silently re-init this server
// workspace as embedded (pkit-zj7y). Falls back to the generic hint for
// embedded or unreadable workspaces.
func storeOpenHint(beadsDir string) string {
	if cfg, err := configfile.Load(beadsDir); err == nil && cfg != nil &&
		strings.ToLower(cfg.DoltMode) == configfile.DoltModeServer {
		return "this workspace uses a Dolt sql-server. Verify the server is running and " +
			"the BEADS_DOLT_* variables are exported in this shell (direnv); try 'bd dolt status'. " +
			"Do NOT run 'bd init' — it would re-initialize this server workspace as embedded."
	}
	return diagHint()
}

// ensureStoreActive guarantees that a storage backend is initialized and tracked.
// Uses the factory to respect metadata.json backend configuration.
func ensureStoreActive() error {
	lockStore()
	active := isStoreActive() && getStore() != nil
	unlockStore()
	if active {
		return nil
	}

	// Find the .beads directory
	beadsDir := beads.FindBeadsDir()
	if beadsDir == "" {
		return fmt.Errorf("no beads database found.\n" +
			"Hint: run 'bd init' to create a database in the current directory")
	}

	// Use the factory to create the appropriate backend
	// based on metadata.json configuration and build tags
	store, err := newDoltStoreFromConfig(getRootContext(), beadsDir)
	if err != nil {
		return fmt.Errorf("failed to open database: %w\nHint: %s", err, storeOpenHint(beadsDir))
	}

	// Update the database path for compatibility with code that expects it
	if dbPath := beads.FindDatabasePath(); dbPath != "" {
		setDBPath(dbPath)
	}

	lockStore()
	setStore(store)
	setStoreActive(true)
	unlockStore()

	return nil
}

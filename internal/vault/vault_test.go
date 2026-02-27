package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitAndOpen(t *testing.T) {
	dir := t.TempDir()

	v, err := Init(dir, "test-project", "test-password")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if !v.IsUnlocked() {
		t.Error("vault should be unlocked after init")
	}

	if v.Config.Project != "test-project" {
		t.Errorf("project = %q, want %q", v.Config.Project, "test-project")
	}

	// Check files exist
	if _, err := os.Stat(filepath.Join(dir, VaultDir, ConfigFile)); err != nil {
		t.Errorf("config.json should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, VaultDir, VaultFile)); err != nil {
		t.Errorf("vault.enc should exist: %v", err)
	}
}

func TestInitDuplicate(t *testing.T) {
	dir := t.TempDir()

	_, err := Init(dir, "test", "pass")
	if err != nil {
		t.Fatal(err)
	}

	_, err = Init(dir, "test", "pass")
	if err == nil {
		t.Error("Init() should fail when vault already exists")
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	if err := v.Set("development", "DB_URL", "postgres://localhost/db"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, err := v.Get("development", "DB_URL")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "postgres://localhost/db" {
		t.Errorf("Get() = %q, want %q", got, "postgres://localhost/db")
	}
}

func TestSetCreatesEnvironment(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	if err := v.Set("staging", "KEY", "val"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	envs := v.ListEnvironments()
	found := false
	for _, e := range envs {
		if e == "staging" {
			found = true
		}
	}
	if !found {
		t.Error("staging environment should be created")
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	v.Set("development", "B_KEY", "val2")
	v.Set("development", "A_KEY", "val1")

	keys, err := v.List("development")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(keys) != 2 {
		t.Fatalf("List() returned %d keys, want 2", len(keys))
	}

	// Should be sorted
	if keys[0] != "A_KEY" || keys[1] != "B_KEY" {
		t.Errorf("List() = %v, want [A_KEY, B_KEY]", keys)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	v.Set("development", "KEY", "val")
	if err := v.Remove("development", "KEY"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	_, err := v.Get("development", "KEY")
	if err == nil {
		t.Error("Get() should fail after Remove()")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	err := v.Remove("development", "NOPE")
	if err == nil {
		t.Error("Remove() should fail for nonexistent key")
	}
}

func TestEnv(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	v.Set("development", "A", "1")
	v.Set("development", "B", "2")

	env, err := v.Env("development")
	if err != nil {
		t.Fatalf("Env() error: %v", err)
	}

	if len(env) != 2 {
		t.Fatalf("Env() returned %d keys, want 2", len(env))
	}
	if env["A"] != "1" || env["B"] != "2" {
		t.Errorf("Env() = %v", env)
	}
}

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	v.Set("development", "API_KEY", "old-key")
	if err := v.Rotate("development", "API_KEY", "new-key"); err != nil {
		t.Fatalf("Rotate() error: %v", err)
	}

	got, _ := v.Get("development", "API_KEY")
	if got != "new-key" {
		t.Errorf("after Rotate(), Get() = %q, want %q", got, "new-key")
	}

	// Check history
	entry := v.data.Environments["development"]["API_KEY"]
	if len(entry.History) != 1 {
		t.Fatalf("history length = %d, want 1", len(entry.History))
	}
	if entry.History[0].Value != "old-key" {
		t.Errorf("history[0].Value = %q, want %q", entry.History[0].Value, "old-key")
	}
}

func TestLockUnlock(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "password123")

	v.Set("development", "SECRET", "s3cret")
	v.Lock()

	if v.IsUnlocked() {
		t.Error("vault should be locked after Lock()")
	}

	// Open and unlock
	v2, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if err := v2.Unlock("password123"); err != nil {
		t.Fatalf("Unlock() error: %v", err)
	}

	got, err := v2.Get("development", "SECRET")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "s3cret" {
		t.Errorf("Get() = %q, want %q", got, "s3cret")
	}
}

func TestUnlockWrongPassword(t *testing.T) {
	dir := t.TempDir()
	Init(dir, "test", "correct-password")

	v2, _ := Open(dir)
	if err := v2.Unlock("wrong-password"); err == nil {
		t.Error("Unlock() with wrong password should error")
	}
}

func TestLockedOperations(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")
	v.Lock()

	if err := v.Set("dev", "k", "v"); err == nil {
		t.Error("Set() on locked vault should error")
	}
	if _, err := v.Get("dev", "k"); err == nil {
		t.Error("Get() on locked vault should error")
	}
	if _, err := v.List("dev"); err == nil {
		t.Error("List() on locked vault should error")
	}
	if err := v.Remove("dev", "k"); err == nil {
		t.Error("Remove() on locked vault should error")
	}
	if _, err := v.Env("dev"); err == nil {
		t.Error("Env() on locked vault should error")
	}
	if err := v.Rotate("dev", "k", "v"); err == nil {
		t.Error("Rotate() on locked vault should error")
	}
}

func TestImportSecrets(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	secrets := map[string]string{
		"DB_URL":  "postgres://localhost/db",
		"API_KEY": "sk-123",
	}

	if err := v.ImportSecrets("development", secrets); err != nil {
		t.Fatalf("ImportSecrets() error: %v", err)
	}

	got, _ := v.Get("development", "DB_URL")
	if got != "postgres://localhost/db" {
		t.Errorf("DB_URL = %q, want %q", got, "postgres://localhost/db")
	}

	got, _ = v.Get("development", "API_KEY")
	if got != "sk-123" {
		t.Errorf("API_KEY = %q, want %q", got, "sk-123")
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()

	if Exists(dir) {
		t.Error("Exists() should be false before Init()")
	}

	Init(dir, "test", "pass")

	if !Exists(dir) {
		t.Error("Exists() should be true after Init()")
	}
}

func TestVaultFilePermissions(t *testing.T) {
	dir := t.TempDir()
	Init(dir, "test", "test-password")

	// Check vault.enc permissions
	vaultFile := filepath.Join(dir, VaultDir, VaultFile)
	info, err := os.Stat(vaultFile)
	if err != nil {
		t.Fatalf("stat vault.enc: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("vault.enc permissions = %o, want 0600", perm)
	}

	// Check config.json permissions
	configFile := filepath.Join(dir, VaultDir, ConfigFile)
	info, err = os.Stat(configFile)
	if err != nil {
		t.Fatalf("stat config.json: %v", err)
	}
	perm = info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("config.json permissions = %o, want 0600", perm)
	}

	// Check .humsafe directory permissions
	vaultDirPath := filepath.Join(dir, VaultDir)
	info, err = os.Stat(vaultDirPath)
	if err != nil {
		t.Fatalf("stat .humsafe: %v", err)
	}
	perm = info.Mode().Perm()
	if perm != 0700 {
		t.Errorf(".humsafe directory permissions = %o, want 0700", perm)
	}
}

func TestChangePassword(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "old-password")

	v.Set("development", "SECRET", "s3cret")

	if err := v.ChangePassword("new-password"); err != nil {
		t.Fatalf("ChangePassword() error: %v", err)
	}
	v.Lock()

	// Re-open with new password
	v2, _ := Open(dir)
	if err := v2.Unlock("new-password"); err != nil {
		t.Fatalf("Unlock with new password: %v", err)
	}

	got, _ := v2.Get("development", "SECRET")
	if got != "s3cret" {
		t.Errorf("Get() = %q, want %q", got, "s3cret")
	}

	// Old password should fail
	v3, _ := Open(dir)
	if err := v3.Unlock("old-password"); err == nil {
		t.Error("Unlock with old password should fail")
	}
}

func TestGetNonexistentEnv(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	_, err := v.Get("production", "KEY")
	if err == nil {
		t.Error("Get() should fail for nonexistent environment")
	}
}

func TestListEnvironments(t *testing.T) {
	dir := t.TempDir()
	v, _ := Init(dir, "test", "pass")

	v.Set("staging", "K", "V")
	v.Set("production", "K", "V")

	envs := v.ListEnvironments()
	if len(envs) < 3 {
		t.Fatalf("ListEnvironments() returned %d, want >= 3", len(envs))
	}
}

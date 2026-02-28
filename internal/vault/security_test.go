package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/georgele/hum/internal/vault/crypto"
)

func TestRotationHistoryCapped(t *testing.T) {
	dir := t.TempDir()
	v, err := Init(dir, "test", "pass")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := v.Set("development", "KEY", "initial"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	for i := 1; i <= 15; i++ {
		if err := v.Rotate("development", "KEY", fmt.Sprintf("val-%d", i)); err != nil {
			t.Fatalf("Rotate() iteration %d error: %v", i, err)
		}
	}

	history := v.data.Environments["development"]["KEY"].History
	if len(history) != MaxRotationHistory {
		t.Errorf("history length = %d, want %d (MaxRotationHistory)", len(history), MaxRotationHistory)
	}
}

func TestRotationHistoryKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	v, err := Init(dir, "test", "pass")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := v.Set("development", "KEY", "initial"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	for i := 1; i <= 15; i++ {
		if err := v.Rotate("development", "KEY", fmt.Sprintf("val-%d", i)); err != nil {
			t.Fatalf("Rotate() iteration %d error: %v", i, err)
		}
	}

	history := v.data.Environments["development"]["KEY"].History
	if len(history) != MaxRotationHistory {
		t.Fatalf("history length = %d, want %d", len(history), MaxRotationHistory)
	}

	// After 15 rotations starting from "initial", the history should contain
	// the 10 most recent previous values. The rotation sequence is:
	//   Rotate #1: old="initial" -> new="val-1"   => history gets "initial"
	//   Rotate #2: old="val-1"   -> new="val-2"   => history gets "val-1"
	//   ...
	//   Rotate #15: old="val-14" -> new="val-15"  => history gets "val-14"
	// Total 15 history entries; only the last 10 are kept: "val-5" through "val-14".
	for i := 0; i < MaxRotationHistory; i++ {
		expected := fmt.Sprintf("val-%d", i+5)
		if history[i].Value != expected {
			t.Errorf("history[%d].Value = %q, want %q", i, history[i].Value, expected)
		}
	}
}

func TestAtomicWriteFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	data := []byte("secret data")

	if err := atomicWriteFile(path, data, 0600); err != nil {
		t.Fatalf("atomicWriteFile() error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestAtomicWriteTempFileCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	data := []byte("some data")

	if err := atomicWriteFile(path, data, 0600); err != nil {
		t.Fatalf("atomicWriteFile() error: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("temp file %s should not exist after successful write", tmpPath)
	}
}

func TestZeroBytesActuallyZeroes(t *testing.T) {
	buf := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}
	crypto.ZeroBytes(buf)

	for i, b := range buf {
		if b != 0 {
			t.Errorf("buf[%d] = 0x%02X, want 0x00", i, b)
		}
	}
}

func TestLockZeroesKeyMaterial(t *testing.T) {
	dir := t.TempDir()
	v, err := Init(dir, "test", "pass")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if v.key == nil {
		t.Fatal("key should be non-nil before Lock()")
	}
	if v.salt == nil {
		t.Fatal("salt should be non-nil before Lock()")
	}

	v.Lock()

	if v.key != nil {
		t.Error("key should be nil after Lock()")
	}
	if v.salt != nil {
		t.Error("salt should be nil after Lock()")
	}
}

func TestChangePasswordZeroesOldKey(t *testing.T) {
	dir := t.TempDir()
	v, err := Init(dir, "test", "pass")
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// Save a reference to the old key's backing slice
	oldKey := v.key
	oldKeySnapshot := make([]byte, len(oldKey))
	copy(oldKeySnapshot, oldKey)

	// Verify old key is non-zero before password change
	allZero := true
	for _, b := range oldKeySnapshot {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("old key should be non-zero before ChangePassword()")
	}

	if err := v.ChangePassword("new-pass"); err != nil {
		t.Fatalf("ChangePassword() error: %v", err)
	}

	// The old key slice (oldKey) should now be zeroed
	for i, b := range oldKey {
		if b != 0 {
			t.Errorf("old key byte[%d] = 0x%02X, want 0x00 (old key was not zeroed)", i, b)
		}
	}
}

package keychain

import (
	"strings"
	"testing"
)

func TestAccountKeyDifferentPaths(t *testing.T) {
	keyA := accountKey("/project/a")
	keyB := accountKey("/project/b")
	if keyA == keyB {
		t.Fatalf("expected different keys for different paths, got %q for both", keyA)
	}
}

func TestAccountKeyConsistent(t *testing.T) {
	key1 := accountKey("/project/a")
	key2 := accountKey("/project/a")
	if key1 != key2 {
		t.Fatalf("expected identical keys for same path, got %q and %q", key1, key2)
	}
}

func TestAccountKeyContainsBaseName(t *testing.T) {
	key := accountKey("/project/myapp")
	if !strings.Contains(key, "myapp") {
		t.Fatalf("expected key to contain 'myapp', got %q", key)
	}
}

func TestAccountKeyPathTraversalSafe(t *testing.T) {
	key := accountKey("../../etc/shadow")
	if strings.Contains(key, "/") {
		t.Fatalf("key should not contain '/', got %q", key)
	}
	if strings.Contains(key, "..") {
		t.Fatalf("key should not contain '..', got %q", key)
	}
}

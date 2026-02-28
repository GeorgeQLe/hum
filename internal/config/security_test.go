package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDependencyOrderSelfCycle(t *testing.T) {
	apps := []App{
		makeApp("A", "A"),
	}

	deps, err := DependencyOrder(apps, "A")
	// A self-cycle means A can't be a dependency of itself.
	// DependencyOrder should either return an error or return an empty list.
	if err != nil {
		return // error is acceptable
	}
	if len(deps) != 0 {
		t.Errorf("expected empty dependency list for self-cycle, got %v", deps)
	}
}

func TestDependencyOrderCircularDeps(t *testing.T) {
	// B→C, C→D, D→B forms a cycle that does not include the target A.
	// DependencyOrder must detect the cycle among A's transitive deps.
	apps := []App{
		makeApp("A", "B"),
		makeApp("B", "C"),
		makeApp("C", "D"),
		makeApp("D", "B"),
	}

	_, err := DependencyOrder(apps, "A")
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected error containing 'cycle', got: %v", err)
	}
}

func TestDependencyOrderTargetNotInOutput(t *testing.T) {
	apps := []App{
		makeApp("A", "B"),
		makeApp("B", "C"),
		makeApp("C"),
	}

	deps, err := DependencyOrder(apps, "A")
	if err != nil {
		t.Fatalf("DependencyOrder: %v", err)
	}

	// Should return ["C", "B"] and NOT include "A"
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(deps), deps)
	}
	for _, d := range deps {
		if d == "A" {
			t.Error("target 'A' should not be in the dependency output")
		}
	}
	// Verify order: C before B
	idx := make(map[string]int)
	for i, d := range deps {
		idx[d] = i
	}
	if idx["C"] > idx["B"] {
		t.Error("C should come before B")
	}
}

func TestTopologicalSortThreeWayCycle(t *testing.T) {
	apps := []App{
		makeApp("A", "B"),
		makeApp("B", "C"),
		makeApp("C", "A"),
	}

	_, err := TopologicalSort(apps)
	if err == nil {
		t.Fatal("expected error for three-way cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected error containing 'cycle', got: %v", err)
	}
}

func TestValidateRejectsControlChars(t *testing.T) {
	app := App{
		Name:    "bad\x1bapp",
		Dir:     ".",
		Command: "echo test",
		Ports:   []int{3000},
	}

	err := app.Validate()
	if err == nil {
		t.Fatal("expected error for name with control characters")
	}
}

func TestValidateRejectsSelfDependency(t *testing.T) {
	apps := []App{
		makeApp("A", "A"),
	}

	err := ValidateDependencies(apps)
	if err == nil {
		t.Fatal("expected error for self-dependency")
	}
}

func TestDuplicateKeysLastWins(t *testing.T) {
	raw := `[{"name":"app","dir":".","command":"echo","ports":[1],"dir":"other"}]`

	var apps []App
	if err := json.Unmarshal([]byte(raw), &apps); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	if apps[0].Dir != "other" {
		t.Errorf("expected dir='other' (last value wins), got %q", apps[0].Dir)
	}
}

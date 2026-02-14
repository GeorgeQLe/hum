package config

import (
	"strings"
	"testing"
)

func makeApp(name string, deps ...string) App {
	return App{
		Name:      name,
		Dir:       ".",
		Command:   "echo " + name,
		Ports:     []int{3000},
		DependsOn: deps,
	}
}

func TestTopologicalSortLinearChain(t *testing.T) {
	// A depends on B, B depends on C → C, B, A
	apps := []App{
		makeApp("A", "B"),
		makeApp("B", "C"),
		makeApp("C"),
	}

	sorted, err := TopologicalSort(apps)
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(sorted))
	}

	// C must come before B, B must come before A
	idx := make(map[string]int)
	for i, a := range sorted {
		idx[a.Name] = i
	}
	if idx["C"] > idx["B"] {
		t.Error("C should come before B")
	}
	if idx["B"] > idx["A"] {
		t.Error("B should come before A")
	}
}

func TestTopologicalSortDiamond(t *testing.T) {
	// A→B, A→C, B→D, C→D
	apps := []App{
		makeApp("A", "B", "C"),
		makeApp("B", "D"),
		makeApp("C", "D"),
		makeApp("D"),
	}

	sorted, err := TopologicalSort(apps)
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}

	if len(sorted) != 4 {
		t.Fatalf("expected 4 apps, got %d", len(sorted))
	}

	idx := make(map[string]int)
	for i, a := range sorted {
		idx[a.Name] = i
	}
	if idx["D"] > idx["B"] {
		t.Error("D should come before B")
	}
	if idx["D"] > idx["C"] {
		t.Error("D should come before C")
	}
	if idx["B"] > idx["A"] {
		t.Error("B should come before A")
	}
	if idx["C"] > idx["A"] {
		t.Error("C should come before A")
	}
}

func TestTopologicalSortCycleDetection(t *testing.T) {
	// A→B, B→A (cycle)
	apps := []App{
		makeApp("A", "B"),
		makeApp("B", "A"),
	}

	_, err := TopologicalSort(apps)
	if err == nil {
		t.Error("expected error for cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func TestTopologicalSortNoDependencies(t *testing.T) {
	apps := []App{
		makeApp("A"),
		makeApp("B"),
		makeApp("C"),
	}

	sorted, err := TopologicalSort(apps)
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("expected 3 apps, got %d", len(sorted))
	}
}

func TestTopologicalSortMissingRef(t *testing.T) {
	// A depends on "missing" which doesn't exist
	// TopologicalSort should still work (missing deps are ignored)
	apps := []App{
		makeApp("A", "missing"),
	}

	sorted, err := TopologicalSort(apps)
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(sorted) != 1 || sorted[0].Name != "A" {
		t.Errorf("expected [A], got %v", sorted)
	}
}

func TestDependencyOrderLinear(t *testing.T) {
	apps := []App{
		makeApp("A", "B"),
		makeApp("B", "C"),
		makeApp("C"),
	}

	deps, err := DependencyOrder(apps, "A")
	if err != nil {
		t.Fatalf("DependencyOrder: %v", err)
	}
	// Should return [C, B] — both must start before A
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d: %v", len(deps), deps)
	}

	idx := make(map[string]int)
	for i, d := range deps {
		idx[d] = i
	}
	if idx["C"] > idx["B"] {
		t.Error("C should come before B in dependency order")
	}
}

func TestDependencyOrderNoDeps(t *testing.T) {
	apps := []App{
		makeApp("A"),
		makeApp("B"),
	}

	deps, err := DependencyOrder(apps, "A")
	if err != nil {
		t.Fatalf("DependencyOrder: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deps, got %v", deps)
	}
}

func TestDependencyOrderDiamond(t *testing.T) {
	apps := []App{
		makeApp("A", "B", "C"),
		makeApp("B", "D"),
		makeApp("C", "D"),
		makeApp("D"),
	}

	deps, err := DependencyOrder(apps, "A")
	if err != nil {
		t.Fatalf("DependencyOrder: %v", err)
	}
	// Should return [D, B, C] or [D, C, B] — D must be first
	if len(deps) != 3 {
		t.Fatalf("expected 3 deps, got %d: %v", len(deps), deps)
	}

	idx := make(map[string]int)
	for i, d := range deps {
		idx[d] = i
	}
	if idx["D"] > idx["B"] {
		t.Error("D should come before B")
	}
	if idx["D"] > idx["C"] {
		t.Error("D should come before C")
	}
}

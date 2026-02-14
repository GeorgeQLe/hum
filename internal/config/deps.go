package config

import "fmt"

// TopologicalSort returns apps in dependency order using Kahn's algorithm.
// Apps with no dependencies come first.
// Returns an error if a cycle is detected.
func TopologicalSort(apps []App) ([]App, error) {
	nameToApp := make(map[string]App, len(apps))
	inDegree := make(map[string]int, len(apps))
	dependents := make(map[string][]string) // dep -> apps that depend on it

	for _, app := range apps {
		nameToApp[app.Name] = app
		inDegree[app.Name] = 0
	}

	for _, app := range apps {
		for _, dep := range app.DependsOn {
			if _, ok := nameToApp[dep]; ok {
				inDegree[app.Name]++
				dependents[dep] = append(dependents[dep], app.Name)
			}
		}
	}

	// Start with nodes that have no dependencies
	var queue []string
	for _, app := range apps {
		if inDegree[app.Name] == 0 {
			queue = append(queue, app.Name)
		}
	}

	var sorted []App
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		sorted = append(sorted, nameToApp[name])

		for _, dep := range dependents[name] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(sorted) != len(apps) {
		return nil, fmt.Errorf("dependency cycle detected")
	}

	return sorted, nil
}

// DependencyOrder returns the list of app names that must be started
// before the target app, in the order they should be started.
// The target itself is not included.
// Returns an error if a dependency cycle is detected.
func DependencyOrder(apps []App, target string) ([]string, error) {
	nameToApp := make(map[string]App, len(apps))
	for _, app := range apps {
		nameToApp[app.Name] = app
	}

	// BFS to collect all transitive dependencies
	visited := make(map[string]bool)
	var collect func(name string)
	collect = func(name string) {
		app, ok := nameToApp[name]
		if !ok {
			return
		}
		for _, dep := range app.DependsOn {
			if !visited[dep] {
				visited[dep] = true
				collect(dep)
			}
		}
	}
	collect(target)

	if len(visited) == 0 {
		return nil, nil
	}

	// Filter apps to just the dependencies and sort them
	var depApps []App
	for _, app := range apps {
		if visited[app.Name] {
			depApps = append(depApps, app)
		}
	}

	sorted, err := TopologicalSort(depApps)
	if err != nil {
		return nil, fmt.Errorf("dependency cycle detected for %q: %w", target, err)
	}

	var names []string
	for _, a := range sorted {
		names = append(names, a.Name)
	}
	return names, nil
}

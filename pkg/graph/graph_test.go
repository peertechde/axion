package graph

import (
	"testing"
)

func TestAddEdge(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() (*Graph, *Node, []*Node) // returns graph, source, targets
		expectError bool
		errorType   error
	}{
		{
			name: "add edge to existing node",
			setup: func() (*Graph, *Node, []*Node) {
				g := New()
				source := NewNode("source")
				target1 := NewNode("target1")
				target2 := NewNode("target2")
				g.AddNode(source, target1, target2)
				return g, source, []*Node{target1, target2}
			},
			expectError: false,
		},
		{
			name: "add edge to non-existent source node",
			setup: func() (*Graph, *Node, []*Node) {
				g := New()
				source := NewNode("source")
				target := NewNode("target")
				g.AddNode(target) // Only add target, not source
				return g, source, []*Node{target}
			},
			expectError: true,
			errorType:   ErrNodeNotFound,
		},
		{
			name: "add edge with empty targets",
			setup: func() (*Graph, *Node, []*Node) {
				g := New()
				source := NewNode("source")
				g.AddNode(source)
				return g, source, []*Node{}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, source, targets := tt.setup()

			err := g.AddEdge(source, targets...)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				if tt.errorType != nil && err != tt.errorType {
					t.Errorf("expected error %v, got %v", tt.errorType, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// Verify edges were added
				if node, exists := g.GetNode(source.Name); exists {
					edges := node.Edges()
					if len(edges) != len(targets) {
						t.Errorf("expected %d edges, got %d", len(targets), len(edges))
					}
				}
			}
		})
	}
}

func TestAddEdgeCacheInvalidation(t *testing.T) {
	g := New()
	nodeA := NewNode("A")
	nodeB := NewNode("B")
	g.AddNode(nodeA, nodeB)

	// Cache the sort order
	_, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error during initial sort: %v", err)
	}

	// Verify cache exists
	g.mu.RLock()
	cached := g.cachedOrder != nil
	g.mu.RUnlock()
	if !cached {
		t.Fatal("expected cache to exist after Sort()")
	}

	// Add edge should invalidate cache
	err = g.AddEdge(nodeA, nodeB)
	if err != nil {
		t.Fatalf("unexpected error adding edge: %v", err)
	}

	// Verify cache was invalidated
	g.mu.RLock()
	cached = g.cachedOrder != nil
	g.mu.RUnlock()
	if cached {
		t.Error("expected cache to be invalidated after AddEdge()")
	}
}

func TestNewNode(t *testing.T) {
	name := "test-node"
	node := NewNode(name)

	if node.Name != name {
		t.Errorf("expected node name %q, got %q", name, node.Name)
	}

	if node.edges == nil {
		t.Error("expected edges slice to be initialized")
	}

	if len(node.edges) != 0 {
		t.Errorf("expected empty edges slice, got length %d", len(node.edges))
	}
}

func TestNodeAddEdge(t *testing.T) {
	source := NewNode("source")
	target1 := NewNode("target1")
	target2 := NewNode("target2")

	source.AddEdge(target1, target2)

	edges := source.Edges()
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}

	// Verify edges are correct
	found1, found2 := false, false
	for _, edge := range edges {
		if edge.Name == "target1" {
			found1 = true
		}
		if edge.Name == "target2" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("not all target nodes found in edges")
	}
}

func TestNodeEdges(t *testing.T) {
	source := NewNode("source")
	target := NewNode("target")
	source.AddEdge(target)

	edges1 := source.Edges()
	edges2 := source.Edges()

	// Verify we get copies, not the same slice
	if &edges1[0] == &edges2[0] {
		t.Error("Edges() should return copies, not references to internal slice")
	}

	// Verify modifying returned slice doesn't affect original
	edges1[0] = NewNode("modified")
	originalEdges := source.Edges()
	if originalEdges[0].Name != "target" {
		t.Error("modifying returned edges slice affected original")
	}
}

func TestAddEdgeByName(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *Graph
		sourceName  string
		targetNames []string
		expectError bool
	}{
		{
			name: "valid edge addition",
			setup: func() *Graph {
				g := New()
				g.AddNode(NewNode("A"), NewNode("B"), NewNode("C"))
				return g
			},
			sourceName:  "A",
			targetNames: []string{"B", "C"},
			expectError: false,
		},
		{
			name: "source node not found",
			setup: func() *Graph {
				g := New()
				g.AddNode(NewNode("B"))
				return g
			},
			sourceName:  "A",
			targetNames: []string{"B"},
			expectError: true,
		},
		{
			name: "target node not found",
			setup: func() *Graph {
				g := New()
				g.AddNode(NewNode("A"))
				return g
			},
			sourceName:  "A",
			targetNames: []string{"B"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := tt.setup()
			err := g.AddEdgeByName(tt.sourceName, tt.targetNames...)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSort(t *testing.T) {
	tests := []struct {
		name          string
		setup         func() *Graph
		expectedOrder []string
		expectError   bool
	}{
		{
			name: "simple linear dependency",
			setup: func() *Graph {
				g := New()
				nodeA := NewNode("A")
				nodeB := NewNode("B")
				nodeC := NewNode("C")
				g.AddNode(nodeA, nodeB, nodeC)
				g.AddEdgeByName("A", "B")
				g.AddEdgeByName("B", "C")
				return g
			},
			expectedOrder: []string{"A", "B", "C"},
			expectError:   false,
		},
		{
			name: "circular dependency",
			setup: func() *Graph {
				g := New()
				nodeA := NewNode("A")
				nodeB := NewNode("B")
				g.AddNode(nodeA, nodeB)
				g.AddEdgeByName("A", "B")
				g.AddEdgeByName("B", "A")
				return g
			},
			expectError: true,
		},
		{
			name: "no dependencies",
			setup: func() *Graph {
				g := New()
				g.AddNode(NewNode("A"), NewNode("B"))
				return g
			},
			expectedOrder: []string{"A", "B"}, // Order may vary but both should be present
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := tt.setup()
			result, err := g.Sort()

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expectedOrder) {
				t.Errorf("expected %d nodes, got %d", len(tt.expectedOrder), len(result))
				return
			}

			// For deterministic tests, check exact order
			if tt.name == "simple linear dependency" {
				for i, node := range result {
					if node.Name != tt.expectedOrder[i] {
						t.Errorf("expected node %q at position %d, got %q", tt.expectedOrder[i], i, node.Name)
					}
				}
			}
		})
	}
}

func TestSortCaching(t *testing.T) {
	g := New()
	nodeA := NewNode("A")
	nodeB := NewNode("B")
	g.AddNode(nodeA, nodeB)

	// First call should compute and cache
	result1, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call should return cached result
	result2, err := g.Sort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Results should be identical (same underlying array)
	if &result1[0] != &result2[0] {
		t.Error("expected cached result to be identical")
	}
}

func TestReversed(t *testing.T) {
	// Create original graph: A -> B -> C
	g := New()
	nodeA := NewNode("A")
	nodeB := NewNode("B")
	nodeC := NewNode("C")
	g.AddNode(nodeA, nodeB, nodeC)
	g.AddEdgeByName("A", "B")
	g.AddEdgeByName("B", "C")

	reversed := g.Reversed()

	// In reversed graph: C -> B -> A
	// Check that C has edge to B
	nodeC_rev, exists := reversed.GetNode("C")
	if !exists {
		t.Fatal("node C not found in reversed graph")
	}
	edges := nodeC_rev.Edges()
	if len(edges) != 1 || edges[0].Name != "B" {
		t.Error("expected C to have edge to B in reversed graph")
	}

	// Check that B has edge to A
	nodeB_rev, exists := reversed.GetNode("B")
	if !exists {
		t.Fatal("node B not found in reversed graph")
	}
	edges = nodeB_rev.Edges()
	if len(edges) != 1 || edges[0].Name != "A" {
		t.Error("expected B to have edge to A in reversed graph")
	}

	// Check that A has no outgoing edges
	nodeA_rev, exists := reversed.GetNode("A")
	if !exists {
		t.Fatal("node A not found in reversed graph")
	}
	edges = nodeA_rev.Edges()
	if len(edges) != 0 {
		t.Error("expected A to have no edges in reversed graph")
	}
}

func TestGetDependents(t *testing.T) {
	g := New()
	nodeA := NewNode("A")
	nodeB := NewNode("B")
	nodeC := NewNode("C")
	g.AddNode(nodeA, nodeB, nodeC)
	g.AddEdgeByName("A", "B", "C")

	dependents := g.GetDependents("A")
	if len(dependents) != 2 {
		t.Errorf("expected 2 dependents, got %d", len(dependents))
	}

	// Check non-existent node
	dependents = g.GetDependents("nonexistent")
	if len(dependents) != 0 {
		t.Errorf("expected 0 dependents for non-existent node, got %d", len(dependents))
	}
}

func TestGetNode(t *testing.T) {
	g := New()
	node := NewNode("test")
	g.AddNode(node)

	// Test existing node
	retrieved, exists := g.GetNode("test")
	if !exists {
		t.Error("expected node to exist")
	}
	if retrieved.Name != "test" {
		t.Errorf("expected node name 'test', got %q", retrieved.Name)
	}

	// Test non-existent node
	_, exists = g.GetNode("nonexistent")
	if exists {
		t.Error("expected node to not exist")
	}
}

func TestNodes(t *testing.T) {
	g := New()
	nodeA := NewNode("A")
	nodeB := NewNode("B")
	g.AddNode(nodeA, nodeB)

	nodes := g.Nodes()
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}

	// Verify all nodes are present
	names := make(map[string]bool)
	for _, node := range nodes {
		names[node.Name] = true
	}

	if !names["A"] || !names["B"] {
		t.Error("not all nodes returned by Nodes()")
	}
}

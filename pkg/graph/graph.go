package graph

import (
	"fmt"
	"io"
	"sort"
	"sync"
)

var (
	// ErrCircularDependency is returned when the graph cannot be topologically sorted
	// because it contains a cycle.
	ErrCircularDependency = fmt.Errorf("circular dependency found")

	// ErrNodeNotFound indicates a requested node does not exist in the graph.
	ErrNodeNotFound = fmt.Errorf("node not found")
)

func New() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
	}
}

// Graph represents a directed graph.
type Graph struct {
	// Nodes stores all nodes in the graph, mapped by their unique name.
	nodes map[string]*Node

	// mu protected the cachedOrder
	mu sync.RWMutex
	// cachedOrder stores the topologically sorted nodes to avoid recomputation
	cachedOrder []*Node
}

// Sort returns the nodes in topological order, caching the result. Subsequent calls
// return the cached result unless Invalidate() is called.
func (g *Graph) Sort() ([]*Node, error) {
	g.mu.RLock()
	if g.cachedOrder != nil {
		defer g.mu.RUnlock()
		return g.cachedOrder, nil
	}
	g.mu.RUnlock()

	g.mu.Lock()
	defer g.mu.Unlock()
	sorted, err := g.sort()
	if err != nil {
		return nil, err
	}

	g.cachedOrder = sorted
	return sorted, nil
}

func (g *Graph) Invalidate() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cachedOrder = nil
}

// NewNode creates a new graph node with the given name.
func NewNode(name string) *Node {
	return &Node{
		Name:  name,
		edges: make([]*Node, 0),
	}
}

// Node represents a node in the graph.
type Node struct {
	Name  string
	edges []*Node
}

// AddEdge adds dependency edges from this node to the target nodes. It means this node
// must come before the target nodes in a topological sort.
func (n *Node) AddEdge(targets ...*Node) {
	n.edges = append(n.edges, targets...)
}

// Edges returns a slice of nodes that this node points to.
func (n *Node) Edges() []*Node {
	edgesCopy := make([]*Node, len(n.edges))
	copy(edgesCopy, n.edges)
	return edgesCopy
}

// AddNode adds one or more nodes to the graph. If a node with the same name already
// exists, it is overwritten.
func (g *Graph) AddNode(nodes ...*Node) {
	for _, node := range nodes {
		g.nodes[node.Name] = node
	}
}

// AddEdge adds dependency edges from a source node to target nodes within the graph. It's
// a convenience method; you could also get the node and call node.AddEdge(). If the
// source node doesn't exist, this is a no-op.
func (g *Graph) AddEdge(source *Node, targets ...*Node) error {
	if _, exists := g.nodes[source.Name]; exists {
		g.nodes[source.Name].AddEdge(targets...)
		g.Invalidate()
		return nil
	}
	return ErrNodeNotFound
}

// AddEdgeByName adds dependency edges from a source node (by name) to target nodes (by
// name). Returns an error if the source or any target node doesn't exist in the graph.
func (g *Graph) AddEdgeByName(sourceName string, targetNames ...string) error {
	sourceNode, ok := g.nodes[sourceName]
	if !ok {
		return fmt.Errorf("source node %q: %w", sourceName, ErrNodeNotFound)
	}

	targets := make([]*Node, len(targetNames))
	for i, targetName := range targetNames {
		targetNode, ok := g.nodes[targetName]
		if !ok {
			return fmt.Errorf("target node %q: %w", targetName, ErrNodeNotFound)
		}
		targets[i] = targetNode
	}

	sourceNode.AddEdge(targets...)
	g.Invalidate()
	return nil
}

// GetNode retrieves a node by its name. The boolean indicates if the node was found.
func (g *Graph) GetNode(name string) (*Node, bool) {
	n, ok := g.nodes[name]
	return n, ok
}

// GetDependents retrieves all nodes that depend on the given node by its name. Returns
// all nodes that the specified node has edges pointing to. If the node doesn't exist,
// returns an empty slice.
func (g *Graph) GetDependents(name string) []*Node {
	node, exists := g.nodes[name]
	if !exists {
		return []*Node{}
	}

	// Return a copy of the edges to prevent external modification
	dependents := make([]*Node, len(node.edges))
	copy(dependents, node.edges)
	return dependents
}

// Nodes returns a slice of all nodes in the graph. Order is not guaranteed.
func (g *Graph) Nodes() []*Node {
	nodes := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

// Sort performs a topological sort of the graph. It returns a slice of nodes in a valid
// order or an error if the graph contains a cycle. This method does not modify the
// original graph.
func (g *Graph) sort() ([]*Node, error) {
	inDegree := make(map[string]int)

	// Use a copy of the adjacency list to avoid modifying original graph nodes. Maps node
	// name to list of neighbor nodes.
	adj := make(map[string][]*Node)

	// Initialize in-degree count and adjacency list copy
	for name, node := range g.nodes {
		// Ensure all nodes start in the maps, even if they have no edges
		if _, exists := inDegree[name]; !exists {
			inDegree[name] = 0
		}
		if _, exists := adj[name]; !exists {
			adj[name] = make([]*Node, 0)
		}
		// Build the adjacency list copy and count in-degrees
		for _, edgeTarget := range node.edges {
			adj[name] = append(adj[name], edgeTarget)
			inDegree[edgeTarget.Name]++
		}
	}

	// Initialize queue with nodes having an in-degree of 0
	queue := make([]*Node, 0)
	for name, degree := range inDegree {
		if degree == 0 {
			node, _ := g.GetNode(name)
			queue = append(queue, node)
		}
	}

	sorted := make([]*Node, 0, len(g.nodes))
	for len(queue) > 0 {
		// Dequeue node n
		n := queue[0]
		queue = queue[1:]

		sorted = append(sorted, n)

		// For each neighbor m of n
		for _, m := range adj[n.Name] {
			// Decrement in-degree of m
			inDegree[m.Name]--
			// If in-degree becomes 0, enqueue m
			if inDegree[m.Name] == 0 {
				queue = append(queue, m)
			}
		}
	}

	// Check for cycles: if sorted list has fewer nodes than the graph, there was a cycle.
	if len(sorted) != len(g.nodes) {
		return nil, ErrCircularDependency
	}

	return sorted, nil
}

// Reversed returns a new graph with all edge directions reversed.
func (g *Graph) Reversed() *Graph {
	reversedGraph := New()
	tempNodes := make(map[string]*Node)

	// Create new nodes for the reversed graph
	for name := range g.nodes {
		newNode := NewNode(name)
		tempNodes[name] = newNode
		reversedGraph.AddNode(newNode)
	}

	// Add reversed edges
	for name, node := range g.nodes {
		originalSourceNode := tempNodes[name]
		for _, edgeTarget := range node.edges {
			// The original target now points to the original source
			reversedTargetNode := tempNodes[edgeTarget.Name]
			reversedTargetNode.AddEdge(originalSourceNode)
		}
	}

	return reversedGraph
}

// AsDot generates a Graphviz DOT representation of the graph.
func (g *Graph) AsDot(w io.Writer, graphName string) {
	fmt.Fprintf(w, "digraph %q {\n", graphName)
	fmt.Fprintf(w, "  rankdir=\"LR\";\n")
	fmt.Fprintf(w, "  node [shape=box, style=rounded];\n")

	// Sort nodes by name for consistent output
	nodeNames := make([]string, 0, len(g.nodes))
	for name := range g.nodes {
		nodeNames = append(nodeNames, name)
	}

	sort.Strings(nodeNames)

	for _, nodeName := range nodeNames {
		node := g.nodes[nodeName]
		if len(node.edges) == 0 {
			fmt.Fprintf(w, "  %q;\n", node.Name)
		} else {
			for _, edge := range node.edges {
				fmt.Fprintf(w, "  %q -> %q;\n", node.Name, edge.Name)
			}
		}
	}
	fmt.Fprintf(w, "}\n")
}

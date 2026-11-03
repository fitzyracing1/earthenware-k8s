// Package tree loads, validates, fills and emits the Earthenware ternary tree
// (tree.json format from github.com/fitzyracing1/earthenware-tree).
//
// Mapping to Kubernetes:
//   - host subtree "C" (kind=computer)  -> control plane / scheduler; receives no workload pods
//   - each VM beginning (VM1..VM4)      -> a node group: nodes labelled earthenware.io/vm=<id>
//   - each block under a VM             -> one pod slot
//
// Slots are filled ternary-tree order: VM beginnings in file order, then steps, then blocks.
package tree

import (
	"encoding/json"
	"fmt"
	"os"
)

const NodeLabel = "earthenware.io/vm"

type Node struct {
	ID     string  `json:"id"`
	Kind   string  `json:"kind"`
	Level  string  `json:"level"`
	Parent *string `json:"parent"`
	Owner  string  `json:"owner"`
	Pod    string  `json:"pod,omitempty"` // emitted: pod occupying this block slot
}

// Tree keeps unknown top-level keys (products, product_links, ...) so output stays tree.json-compatible.
type Tree struct {
	Links map[string]string          `json:"links"`
	Nodes []Node                     `json:"nodes"`
	Extra map[string]json.RawMessage `json:"-"`
}

func Load(path string) (*Tree, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

func Parse(b []byte) (*Tree, error) {
	t := &Tree{}
	if err := json.Unmarshal(b, t); err != nil {
		return nil, err
	}
	raw := map[string]json.RawMessage{}
	_ = json.Unmarshal(b, &raw)
	delete(raw, "links")
	delete(raw, "nodes")
	t.Extra = raw
	return t, t.Validate()
}

func (t *Tree) MarshalJSON() ([]byte, error) {
	m := map[string]any{"links": t.Links, "nodes": t.Nodes}
	for k, v := range t.Extra {
		m[k] = v
	}
	return json.MarshalIndent(m, "", " ")
}

// Validate: ternary (<=3 children), exactly one computer beginning (host),
// every VM beginning linked only to the host.
func (t *Tree) Validate() error {
	children := map[string]int{}
	ids := map[string]bool{}
	host := ""
	for _, n := range t.Nodes {
		ids[n.ID] = true
		if n.Parent != nil {
			children[*n.Parent]++
		} else if n.Kind == "computer" {
			if host != "" {
				return fmt.Errorf("more than one host beginning")
			}
			host = n.ID
		}
	}
	if host == "" {
		return fmt.Errorf("no host (computer) beginning")
	}
	for p, c := range children {
		if !ids[p] {
			return fmt.Errorf("unknown parent %s", p)
		}
		if c > 3 {
			return fmt.Errorf("%s has %d children; ternary max is 3", p, c)
		}
	}
	for _, n := range t.Nodes {
		if n.Parent == nil && n.Kind == "vm" && t.Links[n.ID] != host {
			return fmt.Errorf("VM %s must link only to host %s", n.ID, host)
		}
	}
	return nil
}

func (t *Tree) Counts() (beginnings, steps, blocks int) {
	for _, n := range t.Nodes {
		switch n.Level {
		case "beginning":
			beginnings++
		case "step":
			steps++
		case "block":
			blocks++
		}
	}
	return
}

// Groups returns VM beginning IDs in fill order (host excluded).
func (t *Tree) Groups() []string {
	var g []string
	for _, n := range t.Nodes {
		if n.Parent == nil && n.Kind == "vm" {
			g = append(g, n.ID)
		}
	}
	return g
}

// Slots returns block IDs owned by a VM group, in tree order (step order, then block order).
func (t *Tree) Slots(group string) []string {
	var steps []string
	for _, n := range t.Nodes {
		if n.Parent != nil && *n.Parent == group {
			steps = append(steps, n.ID)
		}
	}
	var s []string
	for _, st := range steps {
		for _, n := range t.Nodes {
			if n.Level == "block" && n.Parent != nil && *n.Parent == st {
				s = append(s, n.ID)
			}
		}
	}
	return s
}

// GroupRank returns fill position of a group (0 = first) or -1 if not a VM group.
func (t *Tree) GroupRank(group string) int {
	for i, g := range t.Groups() {
		if g == group {
			return i
		}
	}
	return -1
}

// Fill assigns pods (in order) to slots ternary-order given pods already placed per group.
// podsByGroup: group -> pods in that group in placement order. Returns a copy with Pod set on blocks.
func (t *Tree) Fill(podsByGroup map[string][]string) (*Tree, []string) {
	out := &Tree{Links: t.Links, Extra: t.Extra, Nodes: append([]Node(nil), t.Nodes...)}
	idx := map[string]int{}
	for i, n := range out.Nodes {
		idx[n.ID] = i
	}
	var overflow []string
	for _, g := range t.Groups() {
		slots := t.Slots(g)
		for i, p := range podsByGroup[g] {
			if i < len(slots) {
				out.Nodes[idx[slots[i]]].Pod = p
			} else {
				overflow = append(overflow, p)
			}
		}
	}
	return out, overflow
}

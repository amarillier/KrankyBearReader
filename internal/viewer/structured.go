package viewer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// DecodeJSON decodes JSON into the generic map/slice/scalar shape shared by the
// structured tree view. json.Number is used (via UseNumber) to preserve
// numeric precision instead of collapsing every number to a lossy float64.
func DecodeJSON(data []byte) (interface{}, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// DecodeYAML decodes YAML into the same generic shape as DecodeJSON.
func DecodeYAML(data []byte) (interface{}, error) {
	var v interface{}
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return normalizeStructured(v), nil
}

// DecodeTOML decodes TOML into the same generic shape as DecodeJSON.
func DecodeTOML(data []byte) (interface{}, error) {
	var v map[string]interface{}
	if _, err := toml.Decode(string(data), &v); err != nil {
		return nil, err
	}
	return normalizeStructured(v), nil
}

// normalizeStructured recursively converts decoder-specific map/slice shapes
// (e.g. yaml.v3's map[interface{}]interface{} on older behavior, or
// BurntSushi/toml's []map[string]interface{} for arrays of tables) into the
// plain map[string]interface{} / []interface{} / scalar shape the tree model
// expects, so it never needs to special-case the source format.
func normalizeStructured(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = normalizeStructured(val)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[fmt.Sprintf("%v", k)] = normalizeStructured(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = normalizeStructured(item)
		}
		return out
	case []map[string]interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = normalizeStructured(item)
		}
		return out
	default:
		return t
	}
}

// structuredNode is one row of the decoded document. label is the object key
// or "[index]" for an array element; value is the underlying scalar/map/slice.
type structuredNode struct {
	label    string
	value    interface{}
	children []widget.TreeNodeID
}

// structuredModel indexes every node by a synthetic ID (not a JSON-path
// string, to sidestep escaping keys containing dots/brackets/quotes).
type structuredModel struct {
	nodes map[widget.TreeNodeID]*structuredNode
	root  []widget.TreeNodeID
}

func buildStructuredModel(data interface{}) *structuredModel {
	m := &structuredModel{nodes: map[widget.TreeNodeID]*structuredNode{}}
	counter := 0

	var add func(label string, value interface{}) widget.TreeNodeID
	add = func(label string, value interface{}) widget.TreeNodeID {
		id := widget.TreeNodeID(fmt.Sprintf("n%d", counter))
		counter++
		n := &structuredNode{label: label, value: value}
		m.nodes[id] = n

		switch v := value.(type) {
		case map[string]interface{}:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				n.children = append(n.children, add(k, v[k]))
			}
		case []interface{}:
			for i, item := range v {
				n.children = append(n.children, add(fmt.Sprintf("[%d]", i), item))
			}
		}
		return id
	}

	switch v := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m.root = append(m.root, add(k, v[k]))
		}
	case []interface{}:
		for i, item := range v {
			m.root = append(m.root, add(fmt.Sprintf("[%d]", i), item))
		}
	default:
		m.root = append(m.root, add("value", v))
	}
	return m
}

func (m *structuredModel) childUIDs(id widget.TreeNodeID) []widget.TreeNodeID {
	if id == "" {
		return m.root
	}
	if n, ok := m.nodes[id]; ok {
		return n.children
	}
	return nil
}

// formatScalar renders a leaf value the way jsonviewer.stack.hu-style tools
// do: quoted strings, bare numbers/bools, "null" for nil.
func formatScalar(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case string:
		return strconv.Quote(t)
	case json.Number:
		return string(t)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func branchSummary(n *structuredNode) string {
	switch n.value.(type) {
	case map[string]interface{}:
		return fmt.Sprintf("%s  {%d}", n.label, len(n.children))
	case []interface{}:
		return fmt.Sprintf("%s  [%d]", n.label, len(n.children))
	default:
		return n.label
	}
}

func leafText(n *structuredNode) string {
	return fmt.Sprintf("%s: %s", n.label, formatScalar(n.value))
}

// nodeMatches reports whether matches accepts this node's own label or
// scalar value text (not its descendants' text).
func nodeMatches(n *structuredNode, matches func(string) bool) bool {
	if matches(n.label) {
		return true
	}
	switch n.value.(type) {
	case map[string]interface{}, []interface{}:
		return false
	default:
		return matches(formatScalar(n.value))
	}
}

// computeVisible returns the set of node IDs to show for a query (every node
// that matches, plus every ancestor needed to reach it from the root) and
// the matching node IDs themselves in tree order, for Next/Prev to cycle
// through.
func computeVisible(m *structuredModel, matches func(string) bool) (visible map[widget.TreeNodeID]bool, matched []widget.TreeNodeID) {
	visible = map[widget.TreeNodeID]bool{}
	var walk func(id widget.TreeNodeID) bool
	walk = func(id widget.TreeNodeID) bool {
		n := m.nodes[id]
		self := nodeMatches(n, matches)
		if self {
			matched = append(matched, id)
		}
		descendantMatch := false
		for _, c := range n.children {
			if walk(c) {
				descendantMatch = true
			}
		}
		if self || descendantMatch {
			visible[id] = true
			return true
		}
		return false
	}
	for _, id := range m.root {
		walk(id)
	}
	return visible, matched
}

// NewStructuredTreeView renders a decoded JSON/YAML/TOML document (from
// DecodeJSON/DecodeYAML/DecodeTOML) as a searchable, collapsible tree.
func NewStructuredTreeView(data interface{}) fyne.CanvasObject {
	model := buildStructuredModel(data)
	var visible map[widget.TreeNodeID]bool // nil means "no filter, show everything"

	childUIDs := func(id widget.TreeNodeID) []widget.TreeNodeID {
		kids := model.childUIDs(id)
		if visible == nil {
			return kids
		}
		out := make([]widget.TreeNodeID, 0, len(kids))
		for _, k := range kids {
			if visible[k] {
				out = append(out, k)
			}
		}
		return out
	}
	isBranch := func(id widget.TreeNodeID) bool {
		return len(childUIDs(id)) > 0
	}

	tree := widget.NewTree(
		childUIDs,
		isBranch,
		func(branch bool) fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.TreeNodeID, branch bool, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			n, ok := model.nodes[id]
			if !ok {
				label.SetText(string(id))
				return
			}
			if branch {
				label.SetText(branchSummary(n))
			} else {
				label.SetText(leafText(n))
			}
		},
	)

	var matched []widget.TreeNodeID
	current := -1

	var bar *findBar
	bar = newFindBar(
		func(query string, useRegex bool) {
			if query == "" {
				visible, matched, current = nil, nil, -1
				bar.SetStatus("")
				tree.Refresh()
				return
			}
			matches, err := compileMatcher(query, useRegex)
			if err != nil {
				bar.SetStatus(err.Error())
				return
			}
			visible, matched = computeVisible(model, matches)
			current = -1
			tree.Refresh()
			tree.OpenAllBranches()
			if len(matched) == 0 {
				bar.SetStatus("no matches")
			} else {
				bar.SetStatus(fmt.Sprintf("%d match(es)", len(matched)))
			}
		},
		func() { // next
			if len(matched) == 0 {
				return
			}
			current = (current + 1) % len(matched)
			tree.Select(matched[current])
			tree.ScrollTo(matched[current])
		},
		func() { // prev
			if len(matched) == 0 {
				return
			}
			current--
			if current < 0 {
				current = len(matched) - 1
			}
			tree.Select(matched[current])
			tree.ScrollTo(matched[current])
		},
	)

	expandAll := widget.NewButton("Expand All", func() { tree.OpenAllBranches() })
	collapseAll := widget.NewButton("Collapse All", func() { tree.CloseAllBranches() })
	toolbar := container.NewBorder(nil, nil, nil, container.NewHBox(expandAll, collapseAll), bar.content)

	return container.NewBorder(toolbar, nil, nil, nil, tree)
}

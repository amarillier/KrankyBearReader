package viewer

import (
	"encoding/xml"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// xmlElement is the generic recursive shape encoding/xml decodes any
// well-formed document into: ",any,attr" collects every attribute regardless
// of name, ",chardata" collects the element's own text (concatenated across
// however many text runs surround its child elements — XML's interleaving
// order between text and child elements is not preserved, since the tree
// view has no use for it), and ",any" recurses into every child element
// regardless of tag. This is XML's own attribute/element/text mix that JSON/
// YAML/TOML's plain map/slice/scalar model (structured.go) has no room for,
// hence a separate model rather than reusing buildStructuredModel.
type xmlElement struct {
	XMLName xml.Name
	Attrs   []xml.Attr   `xml:",any,attr"`
	Content string       `xml:",chardata"`
	Nodes   []xmlElement `xml:",any"`
}

// DecodeXML decodes an XML document's root element into the generic shape
// NewXMLTreeView renders.
func DecodeXML(data []byte) (xmlElement, error) {
	var root xmlElement
	if err := xml.Unmarshal(data, &root); err != nil {
		return xmlElement{}, err
	}
	return root, nil
}

// xmlNodeKind distinguishes the three row shapes a tree node renders as:
// an element (always a branch, or an empty leaf), an attribute, or the
// element's own text content.
type xmlNodeKind int

const (
	xmlKindElement xmlNodeKind = iota
	xmlKindAttr
	xmlKindText
)

// xmlTreeNode is one row of the decoded document.
type xmlTreeNode struct {
	label    string
	kind     xmlNodeKind
	value    string // attribute/text value; unused for elements
	children []widget.TreeNodeID
}

// xmlModel indexes every node by a synthetic ID, mirroring structuredModel.
type xmlModel struct {
	nodes map[widget.TreeNodeID]*xmlTreeNode
	root  []widget.TreeNodeID
}

func buildXMLModel(root xmlElement) *xmlModel {
	m := &xmlModel{nodes: map[widget.TreeNodeID]*xmlTreeNode{}}
	counter := 0
	newID := func() widget.TreeNodeID {
		id := widget.TreeNodeID(fmt.Sprintf("n%d", counter))
		counter++
		return id
	}

	var add func(elem xmlElement) widget.TreeNodeID
	add = func(elem xmlElement) widget.TreeNodeID {
		id := newID()
		n := &xmlTreeNode{label: elem.XMLName.Local, kind: xmlKindElement}
		m.nodes[id] = n

		for _, a := range elem.Attrs {
			aid := newID()
			m.nodes[aid] = &xmlTreeNode{label: "@" + a.Name.Local, kind: xmlKindAttr, value: a.Value}
			n.children = append(n.children, aid)
		}
		if text := strings.TrimSpace(elem.Content); text != "" {
			tid := newID()
			m.nodes[tid] = &xmlTreeNode{label: "#text", kind: xmlKindText, value: text}
			n.children = append(n.children, tid)
		}
		for _, child := range elem.Nodes {
			n.children = append(n.children, add(child))
		}
		return id
	}

	m.root = []widget.TreeNodeID{add(root)}
	return m
}

func (m *xmlModel) childUIDs(id widget.TreeNodeID) []widget.TreeNodeID {
	if id == "" {
		return m.root
	}
	if n, ok := m.nodes[id]; ok {
		return n.children
	}
	return nil
}

// xmlBranchSummary renders an element row: its tag name, plus a child count
// once it has any (attributes, text, and child elements all count, matching
// how structuredModel's branchSummary counts every child alike).
func xmlBranchSummary(n *xmlTreeNode) string {
	if len(n.children) == 0 {
		return "<" + n.label + ">"
	}
	return fmt.Sprintf("<%s>  [%d]", n.label, len(n.children))
}

func xmlLeafText(n *xmlTreeNode) string {
	return fmt.Sprintf("%s: %s", n.label, n.value)
}

// xmlNodeMatches reports whether matches accepts this node's own label or
// value (not its descendants'), mirroring structured.go's nodeMatches.
func xmlNodeMatches(n *xmlTreeNode, matches func(string) bool) bool {
	if matches(n.label) {
		return true
	}
	if n.kind == xmlKindElement {
		return false
	}
	return matches(n.value)
}

// computeVisibleXML mirrors structured.go's computeVisible for the XML model.
func computeVisibleXML(m *xmlModel, matches func(string) bool) (visible map[widget.TreeNodeID]bool, matched []widget.TreeNodeID) {
	visible = map[widget.TreeNodeID]bool{}
	var walk func(id widget.TreeNodeID) bool
	walk = func(id widget.TreeNodeID) bool {
		n := m.nodes[id]
		self := xmlNodeMatches(n, matches)
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

// NewXMLTreeView renders a decoded XML document (from DecodeXML) as a
// searchable, collapsible tree: elements as branches, attributes and text
// content as their own labeled leaf rows underneath.
func NewXMLTreeView(root xmlElement) fyne.CanvasObject {
	model := buildXMLModel(root)
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
	// Deliberately keyed off childUIDs (like structured.go's isBranch) rather
	// than the node's own kind: Fyne's Tree calls isBranch("") on the root
	// sentinel — which has no node of its own — to decide whether the tree
	// has anything to enumerate at all. A kind-based check has no entry for
	// "" and answers false, so the tree never even calls childUIDs("") and
	// renders nothing; counting actual children answers correctly for every
	// id, root included. This also makes a childless (self-closing) element
	// correctly a leaf instead of an empty, dead-end branch arrow.
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
			if n.kind == xmlKindElement {
				label.SetText(xmlBranchSummary(n))
			} else {
				label.SetText(xmlLeafText(n))
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
			visible, matched = computeVisibleXML(model, matches)
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

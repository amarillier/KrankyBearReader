package viewer

import (
	"testing"

	"fyne.io/fyne/v2/widget"
)

func fixtureXML() xmlElement {
	root, err := DecodeXML([]byte(`<person id="1"><name>Ada</name><tags><tag>pioneer</tag><tag>mathematician</tag></tags><meta active="true"></meta></person>`))
	if err != nil {
		panic(err)
	}
	return root
}

func TestDecodeXML_Basic(t *testing.T) {
	root, err := DecodeXML([]byte(`<a x="1">hi</a>`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root.XMLName.Local != "a" {
		t.Errorf("expected root tag 'a', got %q", root.XMLName.Local)
	}
	if len(root.Attrs) != 1 || root.Attrs[0].Value != "1" {
		t.Errorf("expected one attr x=1, got %#v", root.Attrs)
	}
	if root.Content != "hi" {
		t.Errorf("expected content 'hi', got %q", root.Content)
	}
}

func TestDecodeXML_InvalidReturnsError(t *testing.T) {
	if _, err := DecodeXML([]byte(`<a><b></a>`)); err == nil {
		t.Fatal("expected an error for mismatched tags")
	}
}

func TestBuildXMLModel_RootHasAttrAndChildren(t *testing.T) {
	model := buildXMLModel(fixtureXML())
	if len(model.root) != 1 {
		t.Fatalf("expected a single root node, got %d", len(model.root))
	}
	rootID := model.root[0]
	root := model.nodes[rootID]
	if root.label != "person" {
		t.Errorf("expected root label 'person', got %q", root.label)
	}
	// @id attribute, name, tags, meta = 4 children
	if len(root.children) != 4 {
		t.Fatalf("expected 4 children, got %d: %v", len(root.children), root.children)
	}
	attrID := root.children[0]
	attr := model.nodes[attrID]
	if attr.label != "@id" || attr.kind != xmlKindAttr || attr.value != "1" {
		t.Errorf("expected first child to be @id=1 attr, got %#v", attr)
	}
}

func TestBuildXMLModel_TextLeafUnderElement(t *testing.T) {
	model := buildXMLModel(fixtureXML())
	root := model.nodes[model.root[0]]

	var nameID widget.TreeNodeID
	for _, id := range root.children {
		if model.nodes[id].label == "name" {
			nameID = id
		}
	}
	if nameID == "" {
		t.Fatal("expected a 'name' child under root")
	}
	nameChildren := model.childUIDs(nameID)
	if len(nameChildren) != 1 {
		t.Fatalf("expected 1 text child under name, got %d", len(nameChildren))
	}
	text := model.nodes[nameChildren[0]]
	if text.kind != xmlKindText || text.label != "#text" || text.value != "Ada" {
		t.Errorf("expected #text=Ada, got %#v", text)
	}
}

func TestBuildXMLModel_EmptyElementHasNoChildren(t *testing.T) {
	model := buildXMLModel(fixtureXML())
	root := model.nodes[model.root[0]]

	var metaID widget.TreeNodeID
	for _, id := range root.children {
		if model.nodes[id].label == "meta" {
			metaID = id
		}
	}
	if metaID == "" {
		t.Fatal("expected a 'meta' child under root")
	}
	meta := model.nodes[metaID]
	if len(meta.children) != 1 || model.nodes[meta.children[0]].label != "@active" {
		t.Fatalf("expected meta to have a single @active attribute child, got %v", meta.children)
	}
}

func TestComputeVisibleXML_MatchesTextLeafAndKeepsAncestors(t *testing.T) {
	model := buildXMLModel(fixtureXML())
	matches, err := compileMatcher("ada", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	visible, matched := computeVisibleXML(model, matches)
	if len(matched) != 1 {
		t.Fatalf("expected exactly 1 matched node, got %d", len(matched))
	}
	if !visible[model.root[0]] {
		t.Error("expected root to stay visible because a descendant matched")
	}
}

func TestXMLBranchSummaryAndLeafText(t *testing.T) {
	model := buildXMLModel(fixtureXML())
	root := model.nodes[model.root[0]]
	if got := xmlBranchSummary(root); got != "<person>  [4]" {
		t.Errorf("xmlBranchSummary = %q, want %q", got, "<person>  [4]")
	}
	attr := model.nodes[root.children[0]]
	if got := xmlLeafText(attr); got != "@id: 1" {
		t.Errorf("xmlLeafText = %q, want %q", got, "@id: 1")
	}
}

func TestDetectFormat_XMLByExtensionAndSniff(t *testing.T) {
	if got := DetectFormat("doc.xml", nil); got != FormatXML {
		t.Errorf("DetectFormat(doc.xml) = %v, want FormatXML", got)
	}
	if got := DetectFormat("data.unknownext", []byte(`<?xml version="1.0"?><root/>`)); got != FormatXML {
		t.Errorf("sniffed XML declaration = %v, want FormatXML", got)
	}
	if got := DetectFormat("page.unknownext", []byte(`<!DOCTYPE html><html></html>`)); got == FormatXML {
		t.Error("expected an HTML doctype without an XML declaration to not be classified as XML")
	}
}

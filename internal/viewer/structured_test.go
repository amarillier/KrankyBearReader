package viewer

import (
	"encoding/json"
	"testing"

	"fyne.io/fyne/v2/widget"
)

func fixtureData() map[string]interface{} {
	return map[string]interface{}{
		"name": "Ada",
		"age":  json.Number("36"),
		"tags": []interface{}{"pioneer", "mathematician"},
		"meta": map[string]interface{}{"active": true},
	}
}

func TestBuildStructuredModel_TopLevelKeysSortedAsRootChildren(t *testing.T) {
	model := buildStructuredModel(fixtureData())
	if len(model.root) != 4 {
		t.Fatalf("expected 4 root nodes, got %d", len(model.root))
	}
	var labels []string
	for _, id := range model.root {
		labels = append(labels, model.nodes[id].label)
	}
	want := []string{"age", "meta", "name", "tags"}
	for i, w := range want {
		if labels[i] != w {
			t.Errorf("root label[%d] = %q, want %q (labels=%v)", i, labels[i], w, labels)
		}
	}
}

func TestBuildStructuredModel_ChildrenForArraysAndObjects(t *testing.T) {
	model := buildStructuredModel(fixtureData())

	var tagsID, metaID widget.TreeNodeID
	for _, id := range model.root {
		switch model.nodes[id].label {
		case "tags":
			tagsID = id
		case "meta":
			metaID = id
		}
	}

	tagChildren := model.childUIDs(tagsID)
	if len(tagChildren) != 2 {
		t.Fatalf("expected 2 tag children, got %d", len(tagChildren))
	}
	if model.nodes[tagChildren[0]].label != "[0]" || model.nodes[tagChildren[1]].label != "[1]" {
		t.Errorf("unexpected array child labels: %q, %q", model.nodes[tagChildren[0]].label, model.nodes[tagChildren[1]].label)
	}

	metaChildren := model.childUIDs(metaID)
	if len(metaChildren) != 1 || model.nodes[metaChildren[0]].label != "active" {
		t.Fatalf("unexpected meta children: %v", metaChildren)
	}
}

func TestComputeVisible_MatchesLeafAndKeepsAncestors(t *testing.T) {
	model := buildStructuredModel(fixtureData())
	matches, err := compileMatcher("ada", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	visible, _ := computeVisible(model, matches)

	var nameID widget.TreeNodeID
	for _, id := range model.root {
		if model.nodes[id].label == "name" {
			nameID = id
		}
	}
	if !visible[nameID] {
		t.Error("expected the matching 'name' node to be visible")
	}

	for _, id := range model.root {
		if model.nodes[id].label == "tags" && visible[id] {
			t.Error("expected non-matching 'tags' branch to be filtered out")
		}
	}
}

func TestComputeVisible_MatchesNestedLeafKeepsBranchAncestor(t *testing.T) {
	model := buildStructuredModel(fixtureData())
	matches, err := compileMatcher("active", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	visible, matched := computeVisible(model, matches)
	if len(matched) != 1 {
		t.Fatalf("expected exactly 1 matched node, got %d: %v", len(matched), matched)
	}

	var metaID widget.TreeNodeID
	for _, id := range model.root {
		if model.nodes[id].label == "meta" {
			metaID = id
		}
	}
	if !visible[metaID] {
		t.Error("expected 'meta' branch to stay visible because its child matched")
	}
}

func TestFormatScalar(t *testing.T) {
	cases := map[interface{}]string{
		nil:               "null",
		true:              "true",
		"hi":              `"hi"`,
		json.Number("42"): "42",
	}
	for in, want := range cases {
		if got := formatScalar(in); got != want {
			t.Errorf("formatScalar(%#v) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeStructured_ConvertsInterfaceKeyedMaps(t *testing.T) {
	in := map[interface{}]interface{}{"a": 1, "b": []interface{}{map[interface{}]interface{}{"c": 2}}}
	out := normalizeStructured(in)
	m, ok := out.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", out)
	}
	if m["a"] != 1 {
		t.Errorf("expected a=1, got %v", m["a"])
	}
	arr, ok := m["b"].([]interface{})
	if !ok || len(arr) != 1 {
		t.Fatalf("expected b to be a 1-element slice, got %#v", m["b"])
	}
	inner, ok := arr[0].(map[string]interface{})
	if !ok || inner["c"] != 2 {
		t.Fatalf("expected nested map with c=2, got %#v", arr[0])
	}
}

func TestDecodeJSON_PreservesNumberPrecision(t *testing.T) {
	v, err := DecodeJSON([]byte(`{"big": 123456789012345678}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := v.(map[string]interface{})
	if _, ok := m["big"].(json.Number); !ok {
		t.Fatalf("expected json.Number, got %T", m["big"])
	}
}

func TestDecodeYAML_Basic(t *testing.T) {
	v, err := DecodeYAML([]byte("name: Ada\nactive: true\ntags:\n  - a\n  - b\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", v)
	}
	if m["name"] != "Ada" {
		t.Errorf("expected name=Ada, got %v", m["name"])
	}
	tags, ok := m["tags"].([]interface{})
	if !ok || len(tags) != 2 {
		t.Fatalf("expected 2-element tags slice, got %#v", m["tags"])
	}
}

func TestDecodeTOML_Basic(t *testing.T) {
	v, err := DecodeTOML([]byte("name = \"Ada\"\n\n[meta]\nactive = true\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", v)
	}
	meta, ok := m["meta"].(map[string]interface{})
	if !ok || meta["active"] != true {
		t.Fatalf("expected meta.active=true, got %#v", m["meta"])
	}
}

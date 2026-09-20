package configfield

import "testing"

func TestParseNilAndNonStructInputsDoNotPanic(t *testing.T) {
	if got := Parse(nil); got != nil {
		t.Fatalf("Parse(nil) = %v, want nil", got)
	}
	var nilPtr *struct{ A string }
	if got := Parse(nilPtr); got != nil {
		t.Fatalf("Parse(nil pointer) = %v, want nil", got)
	}
	if got := Parse("not a struct"); got != nil {
		t.Fatalf("Parse(string) = %v, want nil", got)
	}
	if got := Parse(42); got != nil {
		t.Fatalf("Parse(int) = %v, want nil", got)
	}
}

func TestParseBasicFields(t *testing.T) {
	type Config struct {
		Host string `json:"host" required:"true" default:"localhost" help:"server host"`
		Port int    `json:"port"`
	}
	items := Parse(&Config{})
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Name != "host" || !items[0].Required || items[0].Default != "localhost" || items[0].Help != "server host" || items[0].Type != "string" {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].Name != "port" || items[1].Required || items[1].Type != "int" {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
}

func TestParseSkipsExplicitlyExcludedFields(t *testing.T) {
	type Config struct {
		Visible string `json:"visible"`
		Hidden  string `json:"-"`
	}
	items := Parse(Config{})
	if len(items) != 1 || items[0].Name != "visible" {
		t.Fatalf("Parse() = %+v, want only the visible field", items)
	}
}

func TestParseSkipsUnexportedFields(t *testing.T) {
	type Config struct {
		Visible string `json:"visible"`
		hidden  string
	}
	items := Parse(Config{hidden: "secret"})
	if len(items) != 1 || items[0].Name != "visible" {
		t.Fatalf("Parse() = %+v, want only the visible field", items)
	}
}

func TestParseFallsBackToKindForUnnamedTypes(t *testing.T) {
	type Config struct {
		Tags  []string          `json:"tags"`
		Extra map[string]string `json:"extra"`
	}
	items := Parse(Config{})
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Type != "slice" {
		t.Fatalf("Tags type = %q, want %q", items[0].Type, "slice")
	}
	if items[1].Type != "map" {
		t.Fatalf("Extra type = %q, want %q", items[1].Type, "map")
	}
}

func TestParseKeepsExplicitAllowedType(t *testing.T) {
	type Config struct {
		Level string `json:"level" type:"option" options:"a,b,c"`
	}
	items := Parse(Config{})
	if len(items) != 1 || items[0].Type != "option" || items[0].Options != "a,b,c" {
		t.Fatalf("unexpected item: %+v", items)
	}
}

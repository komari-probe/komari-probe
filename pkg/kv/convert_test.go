package kv

import (
	"reflect"
	"testing"
)

func TestScanIntegerDefault(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{"empty is zero", "", 0, false},
		{"plain integer", "42", 42, false},
		{"negative integer", "-7", -7, false},
		{"decimal falls back to float and truncates", "1.5", 1, false},
		{"not a number", "abc", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scanIntegerDefault(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("scanIntegerDefault(%q) = %d, nil; want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanIntegerDefault(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("scanIntegerDefault(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseDefaultToFieldBool(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    bool
		wantErr bool
	}{
		{"empty is false", "", false, false},
		{"true", "true", true, false},
		{"false", "false", false, false},
		{"1", "1", true, false},
		{"0", "0", false, false},
		{"invalid word errors instead of silently false", "yes", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var target struct{ V bool }
			field := reflect.ValueOf(&target).Elem().Field(0)
			err := parseDefaultToField(tc.in, field)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDefaultToField(%q) = %v, nil; want error", tc.in, target.V)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDefaultToField(%q) unexpected error: %v", tc.in, err)
			}
			if target.V != tc.want {
				t.Fatalf("parseDefaultToField(%q) = %v, want %v", tc.in, target.V, tc.want)
			}
		})
	}
}

func TestParseDefaultToFieldInt(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int64
		wantErr bool
	}{
		{"empty is zero", "", 0, false},
		{"positive", "42", 42, false},
		{"negative is allowed for signed field", "-5", -5, false},
		{"decimal string", "1.5", 1, false},
		{"garbage errors", "abc", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var target struct{ V int64 }
			field := reflect.ValueOf(&target).Elem().Field(0)
			err := parseDefaultToField(tc.in, field)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDefaultToField(%q) = %d, nil; want error", tc.in, target.V)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDefaultToField(%q) unexpected error: %v", tc.in, err)
			}
			if target.V != tc.want {
				t.Fatalf("parseDefaultToField(%q) = %d, want %d", tc.in, target.V, tc.want)
			}
		})
	}
}

func TestParseDefaultToFieldUint(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    uint64
		wantErr bool
	}{
		{"empty is zero", "", 0, false},
		{"positive", "42", 42, false},
		// Regression test: a negative default on an unsigned field used to be
		// silently converted through float64 into a huge wrapped-around value
		// instead of failing. It must now return an error.
		{"negative on unsigned field errors instead of wrapping", "-5", 0, true},
		{"decimal string", "1.5", 1, false},
		{"garbage errors", "abc", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var target struct{ V uint64 }
			field := reflect.ValueOf(&target).Elem().Field(0)
			err := parseDefaultToField(tc.in, field)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDefaultToField(%q) = %d, nil; want error", tc.in, target.V)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDefaultToField(%q) unexpected error: %v", tc.in, err)
			}
			if target.V != tc.want {
				t.Fatalf("parseDefaultToField(%q) = %d, want %d", tc.in, target.V, tc.want)
			}
		})
	}
}

func TestParseDefaultToFieldFloat(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    float64
		wantErr bool
	}{
		{"empty is zero", "", 0, false},
		{"decimal", "3.14", 3.14, false},
		{"negative decimal", "-2.5", -2.5, false},
		{"garbage errors", "abc", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var target struct{ V float64 }
			field := reflect.ValueOf(&target).Elem().Field(0)
			err := parseDefaultToField(tc.in, field)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDefaultToField(%q) = %v, nil; want error", tc.in, target.V)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDefaultToField(%q) unexpected error: %v", tc.in, err)
			}
			if target.V != tc.want {
				t.Fatalf("parseDefaultToField(%q) = %v, want %v", tc.in, target.V, tc.want)
			}
		})
	}
}

func TestParseDefaultToFieldString(t *testing.T) {
	var target struct{ V string }
	field := reflect.ValueOf(&target).Elem().Field(0)
	if err := parseDefaultToField("hello", field); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.V != "hello" {
		t.Fatalf("got %q, want %q", target.V, "hello")
	}
}

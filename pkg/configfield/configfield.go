package configfield

import (
	"reflect"
	"slices"
)

type Field struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Type     string `json:"type"`
	Options  string `json:"options"`
	Default  string `json:"default"`
	Help     string `json:"help"`
}

var allowTypes = []string{"option", "richtext"}

// Parse reflects v's exported fields into a list of form-field descriptors,
// reading the json/required/type/options/default/help struct tags. v must be
// a struct or a non-nil pointer to one; anything else (including nil)
// yields no items instead of panicking. A field tagged json:"-" is skipped,
// matching encoding/json's convention for excluding a field.
func Parse(v any) []Field {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil
	}

	var items []Field
	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("json")
		if name == "-" {
			continue
		}

		typ := field.Tag.Get("type")
		if !slices.Contains(allowTypes, typ) {
			if typeName := field.Type.Name(); typeName != "" {
				typ = typeName
			} else {
				typ = field.Type.Kind().String()
			}
		}

		items = append(items, Field{
			Name:     name,
			Required: field.Tag.Get("required") == "true",
			Type:     typ,
			Options:  field.Tag.Get("options"),
			Default:  field.Tag.Get("default"),
			Help:     field.Tag.Get("help"),
		})
	}
	return items
}

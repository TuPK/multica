package issueworkflow

import (
	"reflect"
	"strings"
)

// DocTaskFork holds fork-specific task fields.
type DocTaskFork struct {
	// Effects marks work whose result lands outside the platform. "external" holds the
	// task until a person approves it (durable spec 5.4). Empty is ordinary work.
	Effects string `json:"effects,omitempty"`
}

// docFieldTypes collects JSON field names and their Go types for a struct,
// flattening anonymous embedded struct fields without an explicit JSON name
// the same way encoding/json does.
func docFieldTypes(t reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}
	collectDocFieldTypes(t, fields)
	return fields
}

func collectDocFieldTypes(t reflect.Type, fields map[string]reflect.Type) {
	var embedded []reflect.Type
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				embedded = append(embedded, ft)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if _, exists := fields[name]; !exists {
			fields[name] = f.Type
		}
	}
	for _, et := range embedded {
		collectDocFieldTypes(et, fields)
	}
}

package polyjson

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

var codeTemplate = template.Must(template.New("codegen").Funcs(template.FuncMap{
	"lower": strings.ToLower,
}).Parse(
	`// Code generated by polyjson. DO NOT EDIT.

package {{ .Package }}

import (
	"bytes"
	"encoding/json"
	"fmt"
)
{{ range .RawAliases }}
type rawjson{{ . }} {{ . }}
{{ end }}

{{- range $type := .Types }}
// JSON marshaler implementations for {{ .Name }}.

{{- range .Variants }}

func (v {{ . }}) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		rawjson{{ . }}
		Kind string ` + "`json:\"{{ $type.Discriminant }}\"`" + `
	}{rawjson{{ . }}: rawjson{{ . }}(v), Kind: "{{ . }}"})
}

var _ json.Marshaler = {{ . }}{}
{{- end  }}

func Unmarshal{{ .Name }}JSON(b []byte) ({{ .Name }}, error) {
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil, nil
	}

	var probe struct {
		Kind string ` + "`json:\"{{ $type.Discriminant }}\"`" + `
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, fmt.Errorf("unmarshal {{ .Name }} {{ $type.Discriminant }}: %v", err)
	}

	switch probe.Kind {
{{- range .Variants }}
	case "{{ . }}":
		var v {{ . }}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("unmarshal {{ . }}: %v", err)
		}
		return v, nil
{{- end }}
	default:
		return nil, fmt.Errorf("could not unmarshal {{ .Name }} JSON: unknown variant %q", probe.Kind)
	}
}
{{- end }}
{{ range $struct := .Structs }}
// JSON marshaler implementations for {{ .Name }} containing polymorphic fields.

func (v *{{ .Name }}) UnmarshalJSON(b []byte) error {
	var data struct {
		rawjson{{ .Name }}
		{{- range .PolymorphicFields }}

		{{ if eq .Kind "Scalar" }}{{ .Name }} json.RawMessage
		{{- else if eq .Kind "Slice" }}{{ .Name }} []json.RawMessage
		{{- else if eq .Kind "Map" }}{{ .Name }} map[string]json.RawMessage
		{{- end }}
		{{- with .JSONName }}` + " `json:\"{{ . }}\"`" + `{{ end }}
		{{- end }}
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("unmarshal {{ $struct.Name }}: %v", err)
	}
{{ range .PolymorphicFields }}
	{{- if eq .Kind "Scalar" }}
	{{ lower .Name }}Field, err := Unmarshal{{ .Type }}JSON(data.{{ .Name }})
	if err != nil {
		return fmt.Errorf("unmarshal {{ $struct.Name }}.{{ .Name }}: %v", err)
	}
	{{- else if eq .Kind "Slice" }}
	{{ lower .Name }}Field := make([]{{ .Type }}, len(data.{{ .Name }}))
	for i, r := range data.{{ .Name }} {
		v, err := Unmarshal{{ .Type }}JSON(r)
		if err != nil {
			return fmt.Errorf("unmarshal {{ $struct.Name }}.{{ .Name }}[%d]: %v", i, err)
		}
		{{ lower .Name }}Field[i] = v
	}
	{{- else if eq .Kind "Map" }}
	{{ lower .Name }}Field := map[string]{{ .Type }}{}
	for k, r := range data.{{ .Name }} {
		v, err := Unmarshal{{ .Type }}JSON(r)
		if err != nil {
			return fmt.Errorf("unmarshal {{ $struct.Name }}.{{ .Name }}[%s]: %v", k, err)
		}
		{{ lower .Name }}Field[k] = v
	}
	{{- end }}
	{{- end }}

	*v = {{ .Name }}(data.rawjson{{ .Name }})
	{{- range .PolymorphicFields }}
	v.{{ .Name }} = {{ lower .Name }}Field
	{{- end }}
	return nil
}

var _ json.Unmarshaler = &{{ .Name }}{}
{{ end -}}
`))

func WriteMarshalerFile(filename, goPackage string, types []Type, structs []Struct) error {
	if goPackage == "" {
		return fmt.Errorf("a package name is required")
	}
	if len(types) == 0 {
		return fmt.Errorf("no types to generate marshaler for")
	}

	fd, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer fd.Close()

	// Separate out data for a loop that declares the rawjson type aliases.
	// This can not be done in the loops of types and structs because a type
	// referencing itself will create multiple of such rawjson types.
	rawAliases := []string{}
	for _, typ := range types {
		rawAliases = append(rawAliases, typ.Variants...)
	}
	for _, struc := range structs {
		if !containsString(rawAliases, struc.Name) {
			rawAliases = append(rawAliases, struc.Name)
		}
	}

	err = codeTemplate.Execute(fd, struct {
		Package    string
		Types      []Type
		Structs    []Struct
		RawAliases []string
	}{Package: goPackage, Types: types, Structs: structs, RawAliases: rawAliases})
	return err
}

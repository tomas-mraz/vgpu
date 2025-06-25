// Copyright (c) 2023, Cogent Core. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Based on http://github.com/dmarkham/enumer and
// golang.org/x/tools/cmd/stringer:

// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package enumgen

import (
	"strings"
	"text/template"
)

// BuildString builds the string function using a map access approach.
func (g *Generator) BuildString(values []Value, typ *Type) {
	g.Printf("\n")
	g.Printf("\nvar _%sMap = map[%s]string{", typ.Name, typ.Name)
	n := 0
	for _, value := range values {
		g.Printf("%s: `%s`,", &value, value.Name)
		n += len(value.Name)
	}
	g.Printf("}\n\n")
	if typ.IsBitFlag {
		g.ExecTmpl(StringMethodBitFlagTmpl, typ)
	}
	g.ExecTmpl(StringMethodMapTmpl, typ)
}

var StringMethodMapTmpl = template.Must(template.New("StringMethodMap").Parse(
	`{{if .IsBitFlag}}
	// BitIndexString returns the string representation of this {{.Name}} value
	// if it is a bit index value (typically an enum constant), and
	// not an actual bit flag value.
	{{- else}}
	// String returns the string representation of this {{.Name}} value.
	{{- end}}
func (i {{.Name}}) {{if .IsBitFlag}} BitIndexString {{else}} String {{end}} () string { return enums.
	{{- if eq .Extends ""}}String {{else -}} {{if .IsBitFlag -}} BitIndexStringExtended {{else -}} StringExtended {{- end}}[{{.Name}}, {{.Extends}}]{{- end}}(i, _{{.Name}}Map) }
`))

var NConstantTmpl = template.Must(template.New("StringNConstant").Parse(
	`//{{.Name}}N is the highest valid value for type {{.Name}}, plus one.
const {{.Name}}N {{.Name}} = {{.MaxValueP1}}
`))

var NConstantTmplGosl = template.Must(template.New("StringNConstant").Parse(
	`//gosl:start
//{{.Name}}N is the highest valid value for type {{.Name}}, plus one.
const {{.Name}}N {{.Name}} = {{.MaxValueP1}}
//gosl:end
`))

var SetStringMethodTmpl = template.Must(template.New("SetStringMethod").Parse(
	`// SetString sets the {{.Name}} value from its string representation,
// and returns an error if the string is invalid.
func (i *{{.Name}}) SetString(s string) error {
	{{- if eq .Extends ""}} return enums.SetString{{if .Config.AcceptLower}}Lower{{end}}(i, s, _{{.Name}}ValueMap, "{{.Name}}")
	{{- else}} return enums.SetString{{if .Config.AcceptLower}}Lower{{end}}Extended(i, (*{{.Extends}})(i), s, _{{.Name}}ValueMap) {{end}} }
`))

var Int64MethodTmpl = template.Must(template.New("Int64Method").Parse(
	`// Int64 returns the {{.Name}} value as an int64.
func (i {{.Name}}) Int64() int64 { return int64(i) }
`))

var SetInt64MethodTmpl = template.Must(template.New("SetInt64Method").Parse(
	`// SetInt64 sets the {{.Name}} value from an int64.
func (i *{{.Name}}) SetInt64(in int64) { *i = {{.Name}}(in) }
`))

var DescMethodTmpl = template.Must(template.New("DescMethod").Parse(`// Desc returns the description of the {{.Name}} value.
func (i {{.Name}}) Desc() string {
	{{- if eq .Extends ""}} return enums.Desc(i, _{{.Name}}DescMap)
	{{- else}} return enums.DescExtended[{{.Name}}, {{.Extends}}](i, _{{.Name}}DescMap) {{end}} }
`))

var ValuesGlobalTmpl = template.Must(template.New("ValuesGlobal").Parse(
	`// {{.Name}}Values returns all possible values for the type {{.Name}}.
func {{.Name}}Values() []{{.Name}} {
	{{- if eq .Extends ""}} return _{{.Name}}Values
	{{- else}} return enums.ValuesGlobalExtended(_{{.Name}}Values, {{.Extends}}Values())
	{{- end}} }
`))

var ValuesMethodTmpl = template.Must(template.New("ValuesMethod").Parse(
	`// Values returns all possible values for the type {{.Name}}.
func (i {{.Name}}) Values() []enums.Enum {
	{{- if eq .Extends ""}} return enums.Values(_{{.Name}}Values)
	{{- else}} return enums.ValuesExtended(_{{.Name}}Values, {{.Extends}}Values())
	{{- end}} }
`))

var IsValidMethodMapTmpl = template.Must(template.New("IsValidMethodMap").Parse(
	`// IsValid returns whether the value is a valid option for type {{.Name}}.
func (i {{.Name}}) IsValid() bool { _, ok := _{{.Name}}Map[i]; return ok
	{{- if ne .Extends ""}} || {{.Extends}}(i).IsValid() {{end}} }
`))

// BuildBasicMethods builds methods common to all types, like Desc and SetString.
func (g *Generator) BuildBasicMethods(values []Value, typ *Type) {

	// Print the slice of values
	max := int64(0)
	g.Printf("\nvar _%sValues = []%s{", typ.Name, typ.Name)
	for _, value := range values {
		g.Printf("%s, ", &value)
		if value.Value > max {
			max = value.Value
		}
	}
	g.Printf("}\n\n")

	typ.MaxValueP1 = max + 1

	if g.Config.Gosl {
		g.ExecTmpl(NConstantTmplGosl, typ)
	} else {
		g.ExecTmpl(NConstantTmpl, typ)
	}

	// Print the map between name and value
	g.PrintValueMap(values, typ)

	// Print the map of values to descriptions
	g.PrintDescMap(values, typ)

	g.BuildString(values, typ)

	// Print the basic extra methods
	if typ.IsBitFlag {
		g.ExecTmpl(SetStringMethodBitFlagTmpl, typ)
		g.ExecTmpl(SetStringOrMethodBitFlagTmpl, typ)
	} else {
		g.ExecTmpl(SetStringMethodTmpl, typ)
	}
	g.ExecTmpl(Int64MethodTmpl, typ)
	g.ExecTmpl(SetInt64MethodTmpl, typ)
	g.ExecTmpl(DescMethodTmpl, typ)
	g.ExecTmpl(ValuesGlobalTmpl, typ)
	g.ExecTmpl(ValuesMethodTmpl, typ)
	if typ.Config.IsValid {
		g.ExecTmpl(IsValidMethodMapTmpl, typ)
	}
}

// PrintValueMap prints the map between name and value
func (g *Generator) PrintValueMap(values []Value, typ *Type) {
	g.Printf("\nvar _%sValueMap = map[string]%s{", typ.Name, typ.Name)
	for _, value := range values {
		g.Printf("`%s`: %s,", value.Name, &value)
		if typ.Config.AcceptLower {
			l := strings.ToLower(value.Name)
			if l != value.Name { // avoid duplicate keys
				g.Printf("`%s`: %s,", l, &value)
			}
		}
	}
	g.Printf("}\n\n")
}

// PrintDescMap prints the map of values to descriptions
func (g *Generator) PrintDescMap(values []Value, typ *Type) {
	g.Printf("\n")
	g.Printf("\nvar _%sDescMap = map[%s]string{", typ.Name, typ.Name)
	i := 0
	for _, value := range values {
		g.Printf("%s: `%s`,", &value, value.Desc)
		i++
	}
	g.Printf("}\n\n")
}

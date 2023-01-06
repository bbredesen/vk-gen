package def

import (
	"fmt"
	"io"

	"github.com/antchfx/xmlquery"
	"github.com/tidwall/gjson"
)

type staticType struct {
	genericType
}

type primitiveType struct {
	staticType
}

func NewStaticTypeFromXML(node *xmlquery.Node) TypeDefiner {
	rval := staticType{}
	rval.registryName = node.SelectAttr("name")
	rval.publicName = renameIdentifier(rval.registryName)
	rval.internalName = "struct{}"
	rval.byteWidth = 0
	rval.isResolved = true

	return &rval
}

func NewStaticTypeFromJSON(key, json gjson.Result) TypeDefiner {
	if json.Get("primitive").Bool() {
		return NewPrimitiveType(key.String(), json.Get("goType").String(), int(json.Get("byteWidth").Int()))
	}

	rval := staticType{}
	rval.registryName = key.String()
	rval.publicName = renameIdentifier(rval.registryName)
	rval.internalName = json.Get("goType").String()
	rval.byteWidth = int(json.Get("byteWidth").Int())
	rval.isResolved = true

	return &rval
}

func (t *staticType) Category() TypeCategory { return CatStatic }

func (t *staticType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet { return nil }

func (t *staticType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	if t.comment != "" {
		fmt.Fprintln(w, "// ", t.comment)
	}
	fmt.Fprintf(w, "  type %s = %s\n", t.PublicName(), t.InternalName())
}

func (t *staticType) PrintInternalDeclaration(w io.Writer) { /* NOP */ }

func NewPrimitiveType(registryName, goType string, byteWidth int) TypeDefiner {
	rval := primitiveType{}

	rval.registryName = registryName
	rval.publicName = goType
	rval.internalName = goType
	rval.byteWidth = byteWidth
	rval.isResolved = true

	return &rval
}

func (t *primitiveType) Category() TypeCategory { return CatPrimitive }

func (t *primitiveType) PrintPublicDeclaration(w io.Writer) {
	if t.comment != "" {
		fmt.Fprintln(w, "// ", t.comment)
	}
	// No type declaration for primitives
	t.PrintConstValues(w)

}

package def

import (
	"fmt"
	"io"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/tidwall/gjson"
)

type unionType struct {
	structType

	internalByteSize string
}

func (t *unionType) Category() TypeCategory { return CatUnion }

func (t *unionType) IsIdenticalPublicAndInternal() bool { return false }

func (t *unionType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	is := t.structType.Resolve(tr, vr)
	// for _, m := range t.members {
	// 	m.publicName = strcase.ToLowerCamel(m.publicName)
	// }

	is.ResolvedTypes[t.registryName] = t
	t.isResolved = true

	return is
}

func (t *unionType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	fmt.Fprintf(w, "type %s struct {\n", t.PublicName())

	for _, m := range t.members {
		m.PrintPublicDeclaration(w)
		fmt.Fprintf(w, "as%s bool\n", m.PublicName())
	}

	fmt.Fprintf(w, "}\n\n")

	for i, m := range t.members {
		if m.pointerDepth > 0 && m.resolvedType.PublicName() != "string" {
			if m.resolvedType.PublicName() == "unsafe.Pointer" {
				fmt.Fprintf(w, "func (u *%s) As%s(ptr %s) {\n",
					t.PublicName(), m.PublicName(), m.resolvedType.PublicName(),
				)
				fmt.Fprintf(w, "  u.%s = ptr\n", m.PublicName())
			} else {
				fmt.Fprintf(w, "func (u *%s) As%s(vals []%s) {\n",
					t.PublicName(), m.PublicName(), m.resolvedType.PublicName(),
				)
				fmt.Fprintf(w, "  copy(u.%s[:], vals)\n", m.PublicName())
			}
		} else {
			fmt.Fprintf(w, "func (u *%s) As%s(val %s) {\n",
				t.PublicName(), m.PublicName(), m.resolvedType.PublicName(),
			)
			fmt.Fprintf(w, "  u.%s = val\n", m.PublicName())
		}

		for j, n := range t.members {
			fmt.Fprintf(w, "  u.as%s = %v\n", n.PublicName(), i == j)
		}

		fmt.Fprintf(w, "}\n\n")
	}
}

func (t *unionType) PrintInternalDeclaration(w io.Writer) {

	var preamble, structDecl, epilogue strings.Builder
	if t.isReturnedOnly {
		fmt.Fprintf(w, "// WARNING - union %s is returned only, which is not yet handled in the binding\n", t.PublicName())
	}

	// _vk type declaration
	var sizeString = t.internalByteSize
	if t.internalByteSize == "" {
		sizeString = fmt.Sprintf("unsafe.Sizeof(%s{})", t.members[0].resolvedType.InternalName())
	}

	fmt.Fprintf(w, "type %s [%s]byte\n", t.InternalName(), sizeString)

	fmt.Fprintf(w, "func (u *%s) Vulkanize() *%s {\n", t.PublicName(), t.InternalName())
	fmt.Fprintf(w, "  switch true {\n")
	// return (*_vkClearDepthStencilValue)(unsafe.Pointer(s))
	for _, m := range t.members {
		fmt.Fprintf(w, "    case u.as%s:\n", m.PublicName())
		fmt.Fprintf(w, "    return (*%s)(unsafe.Pointer(&u.%s))\n", t.InternalName(), m.PublicName())
		// TODO should be tested but I think there is a bug here. If I have a union with mixed 32 and 64 bit types, and I cast a
		// 32 bit field as 64 bits (as an 8 byte array), will the field be in the most significant bits of the array?

	}
	fmt.Fprintf(w, "    default:\nreturn &%s{}\n", t.InternalName())
	fmt.Fprintf(w, "  }\n")
	fmt.Fprintf(w, "}\n")

	// fmt.Fprintf(w, "func (u *%s) Goify() %s {\n", t.InternalName(), t.PublicName())
	// fmt.Fprintf(w, "  panic(\"Cannot Goify to a Vulkan union type!\")\n")
	// fmt.Fprintf(w, "}\n")

	fmt.Fprint(w, preamble.String(), structDecl.String(), epilogue.String())

	// Goify declaration (if applicable?)
}

func (t *unionType) TranslateToInternal(inputVar string) string {
	if t.IsIdenticalPublicAndInternal() {
		return fmt.Sprintf("%s(%s)", t.InternalName(), inputVar)
	} else {
		return fmt.Sprintf("*%s.Vulkanize()", inputVar)
	}
}

func ReadUnionTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//type[@category='union']") {
		s := newUnionTypeFromXML(node)
		tr[s.RegistryName()] = s
	}
}

func newUnionTypeFromXML(node *xmlquery.Node) *unionType {
	rval := unionType{}

	rval.registryName = node.SelectAttr("name")
	rval.isReturnedOnly = node.SelectAttr("returnedonly") == "true"

	for _, mNode := range xmlquery.Find(node, "member") {
		rval.members = append(rval.members, newStructMemberFromXML(mNode))
	}

	return &rval
}

func ReadUnionExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("union").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
			return true
		} // Ignore comments

		UpdateUnionTypeFromJSON(key, exVal, tr[key.String()].(*unionType))
		// tr[key.String()] = entry

		return true
	})

}

func UpdateUnionTypeFromJSON(key, json gjson.Result, td *unionType) {
	td.internalByteSize = json.Get("go:internalSize").String()
}

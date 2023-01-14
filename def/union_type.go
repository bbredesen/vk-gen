package def

import (
	"fmt"
	"io"
	"strings"

	"github.com/antchfx/xmlquery"
)

type unionType struct {
	structType
}

func (t *unionType) Category() TypeCategory { return CatUnion }

func (t *unionType) IsIdenticalPublicAndInternal() bool { return false }

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
		fmt.Fprintf(w, "// WARNING - struct %s is returned only, which is not yet handled in the binding\n", t.PublicName())
	}
	// _vk type declaration
	fmt.Fprintf(w, "type %s unsafe.Pointer\n", t.InternalName())

	fmt.Fprintf(w, "func (u *%s) Vulkanize() %s {\n", t.PublicName(), t.InternalName())
	fmt.Fprintf(w, "  switch true {\n")
	// return (*_vkClearDepthStencilValue)(unsafe.Pointer(s))
	for _, m := range t.members {
		fmt.Fprintf(w, "    case u.as%s:\n", m.PublicName())
		fmt.Fprintf(w, "    return %s(&u.%s)\n", t.InternalName(), m.PublicName())
	}
	fmt.Fprintf(w, "    default:\nreturn nil\n")
	fmt.Fprintf(w, "  }\n")
	fmt.Fprintf(w, "}\n")

	fmt.Fprint(w, preamble.String(), structDecl.String(), epilogue.String())

	// Goify declaration (if applicable?)
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

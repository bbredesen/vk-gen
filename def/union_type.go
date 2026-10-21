package def

import (
	"fmt"
	"io"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type unionType struct {
	structType

	internalByteSize string

	// selectorType is the type of the sibling member (in the containing struct) that determines which union member is
	// valid when the union is returned by Vulkan. selectorOwner is "Struct.Member" for that sibling, used for docs.
	selectorType  TypeDefiner
	selectorOwner string

	// Set by exceptions.json for unions that have no selector in the registry (see VkPerformanceCounterResultKHR).
	// The union then has no containing struct, so a standalone FromBytes function is generated.
	exceptionSelectorName string
	exceptionSelections   map[string]string // union member registry name -> selector enum value registry name
}

// canGoify is true when the union is selected by a sibling member and every union member can be read back as a
// primitive or string. Struct, union, and array members are not yet supported; however, they are not returned in any unions
// as of Vulkan 1.3, and can be ignored for now. Recursive Goificiation may be needed if that changes in the future.
func (t *unionType) canGoify() bool {
	if t.selectorType == nil {
		return false
	}
	for _, m := range t.members {
		if m.resolvedSelection == nil {
			return false
		}
		switch m.resolvedType.Category() {
		case CatStruct, CatUnion, CatArray:
			return false
		}
	}
	return true
}

func (m *structMember) isCString() bool {
	return m.pointerDepth > 0 && m.resolvedType.PublicName() == "string"
}

func (t *unionType) Category() TypeCategory { return CatUnion }

func (t *unionType) IsIdenticalPublicAndInternal() bool { return false }

func (t *unionType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	if t.exceptionSelectorName != "" {
		for _, m := range t.members {
			m.selection = t.exceptionSelections[m.registryName]
		}
	}

	is := t.structType.Resolve(tr, vr)

	if t.exceptionSelectorName != "" {
		if st := tr[t.exceptionSelectorName]; st != nil {
			is.MergeWith(st.Resolve(tr, vr))
			t.selectorType = st
		} else {
			logrus.WithField("registry type", t.registryName).WithField("selector", t.exceptionSelectorName).
				Error("go:selector type not found in registry")
		}
	}

	is.ResolvedTypes[t.registryName] = t
	t.isResolved = true

	return is
}

func (t *unionType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	if t.canGoify() {
		if t.selectorOwner != "" {
			fmt.Fprintf(w, "//\n// When returned by Vulkan, exactly one field is populated, selected by %s:\n", t.selectorOwner)
		} else {
			fmt.Fprintf(w, "//\n// Exactly one field is valid when returned by Vulkan, selected by a %s value that you supply (see %sFromBytes):\n",
				t.selectorType.PublicName(), t.PublicName())
		}
		for _, m := range t.members {
			fmt.Fprintf(w, "//   %s -> %s\n", m.resolvedSelection.PublicName(), m.PublicName())
		}
		fmt.Fprintf(w, "//\n// Use the Is<Field> methods or switch on the selector to determine which field is valid.\n")
	}
	fmt.Fprintf(w, "type %s struct {\n", t.PublicName())

	for _, m := range t.members {
		m.PrintPublicDeclaration(w)
		fmt.Fprintf(w, "as%s bool\n", m.PublicName())
	}

	fmt.Fprintf(w, "}\n\n")

	for i, m := range t.members {
		if m.pointerDepth > 0 && m.resolvedType.PublicName() != "string" {
			if m.resolvedType.PublicName() == "unsafe.Pointer" || m.resolvedType.Category() == CatPointer { // bugfix/issue-16
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

	for _, m := range t.members {
		fmt.Fprintf(w, "// Is%s is true if %s is the active field of the union.\n", m.PublicName(), m.PublicName())
		fmt.Fprintf(w, "func (u *%s) Is%s() bool { return u.as%s }\n\n", t.PublicName(), m.PublicName(), m.PublicName())
	}
}

func (t *unionType) PrintInternalDeclaration(w io.Writer) {

	var preamble, structDecl, epilogue strings.Builder
	if t.isReturnedOnly && !t.canGoify() {
		logrus.WithField("registry type", t.registryName).
			Error("union is returned only, which is not yet handled in the binding")
	}

	// _vk type declaration
	var sizeString = t.internalByteSize
	if t.internalByteSize == "" {
		switch t.members[0].resolvedType.Category() { // updated with bugfix/issue-16
		case CatPointer:
			sizeString = fmt.Sprintf("unsafe.Sizeof((%s)(nil))", t.members[0].resolvedType.InternalName()) // Internal name will include the pointer and the underlying type
		case CatArray:
			fallthrough // Array will use the same syntax as structs
		case CatStruct:
			sizeString = fmt.Sprintf("unsafe.Sizeof(%s{})", t.members[0].resolvedType.InternalName())
		default:
			sizeString = fmt.Sprintf("unsafe.Sizeof(%s(0))", t.members[0].resolvedType.InternalName()) // fallthrough to assmption that this is a primitve type

		}
	}

	fmt.Fprintf(w, "type %s [%s]byte\n", t.InternalName(), sizeString)

	fmt.Fprintf(w, "func (u *%s) Vulkanize() *%s {\n", t.PublicName(), t.InternalName())
	fmt.Fprintf(w, "  switch true {\n")

	for _, m := range t.members {
		fmt.Fprintf(w, "    case u.as%s:\n", m.PublicName())
		fmt.Fprintf(w, "    return (*%s)(unsafe.Pointer(&u.%s))\n", t.InternalName(), m.PublicName())
		// TODO should be tested but I think there is a bug here. If I have a union with mixed 32 and 64 bit types, and I cast a
		// 32 bit field as 64 bits (as an 8 byte array), will the field be in the most significant bits of the array?
		// Where does Vulkan/C expect them to be?

	}
	fmt.Fprintf(w, "    default:\nreturn &%s{}\n", t.InternalName())
	fmt.Fprintf(w, "  }\n")
	fmt.Fprintf(w, "}\n")

	if t.canGoify() {
		fmt.Fprintf(&epilogue, "func (u *%s) Goify(selector %s) *%s {\n", t.InternalName(), t.selectorType.PublicName(), t.PublicName())
		fmt.Fprintf(&epilogue, "  rval := &%s{}\n  switch selector {\n", t.PublicName())
		for _, m := range t.members {
			fmt.Fprintf(&epilogue, "  case %s:\n", m.resolvedSelection.PublicName())
			if m.isCString() {
				fmt.Fprintf(&epilogue, "    rval.%s = cStringToString(*(*unsafe.Pointer)(unsafe.Pointer(u)))\n", m.PublicName())
			} else {
				fmt.Fprintf(&epilogue, "    rval.%s = %s\n", m.PublicName(),
					m.resolvedType.TranslateToPublic(fmt.Sprintf("*(*%s)(unsafe.Pointer(u))", m.resolvedType.InternalName())))
			}
			fmt.Fprintf(&epilogue, "    rval.as%s = true\n", m.PublicName())
		}
		fmt.Fprintf(&epilogue, "  }\n  return rval\n}\n")

		if t.exceptionSelectorName != "" {
			fmt.Fprintf(&epilogue, "\n// %sFromBytes interprets the leading bytes of b (for example, an element of a result buffer) as the member of\n", t.PublicName())
			fmt.Fprintf(&epilogue, "// the union chosen by selector. b must be at least as long as the largest member of the union.\n")
			fmt.Fprintf(&epilogue, "func %sFromBytes(b []byte, selector %s) *%s {\n", t.PublicName(), t.selectorType.PublicName(), t.PublicName())
			fmt.Fprintf(&epilogue, "  var u %s\n  copy(u[:], b)\n  return u.Goify(selector)\n}\n", t.InternalName())
		}
	}

	// Don't attempt to Goify for now. There may be a command that returns a union, how to handle that TBD
	// fmt.Fprintf(w, "func (u *%s) Goify() %s {\n", t.InternalName(), t.PublicName())
	// fmt.Fprintf(w, "  panic(\"Cannot Goify to a Vulkan union type!\")\n")
	// fmt.Fprintf(w, "}\n")

	fmt.Fprint(w, preamble.String(), structDecl.String(), epilogue.String())
}

func (t *unionType) TranslateToInternal(inputVar string) string {
	if t.IsIdenticalPublicAndInternal() {
		return fmt.Sprintf("%s(%s)", t.InternalName(), inputVar)
	} else {
		return fmt.Sprintf("*%s.Vulkanize()", inputVar)
	}
}

func ReadUnionTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry, api string) {
	queryString := fmt.Sprintf("//types/type[@category='union' and (@api='%s' or not(@api))]", api)

	for _, node := range xmlquery.Find(doc, queryString) {
		s := newUnionTypeFromXML(node, api)
		tr[s.RegistryName()] = s
	}
}

func newUnionTypeFromXML(node *xmlquery.Node, api string) *unionType {
	rval := unionType{}

	rval.registryName = node.SelectAttr("name")
	rval.isReturnedOnly = node.SelectAttr("returnedonly") == "true"

	queryString := fmt.Sprintf("member[@api='%s' or not(@api)]", api)
	for _, mNode := range xmlquery.Find(node, queryString) {
		rval.members = append(rval.members, newStructMemberFromXML(mNode))
	}

	return &rval
}

func ReadUnionExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("union").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
			return true
		} // Ignore comments

		if entry, found := tr[key.String()]; !found {
			logrus.WithField("registry type", key.String()).
				Warn("no existing registry type for union exception")
		} else {
			if entry.Category() != CatUnion {
				logrus.WithField("registry type", key.String()).
					WithField("category", entry.Category().String()).
					Error("exception for union type was not a union in the registry")
			}

			UpdateUnionTypeFromJSON(key, exVal, entry.(*unionType))
		}

		return true
	})

}

func UpdateUnionTypeFromJSON(key, json gjson.Result, td *unionType) {
	td.internalByteSize = json.Get("go:internalSize").String()

	td.exceptionSelectorName = json.Get("go:selector").String()
	json.Get("go:selections").ForEach(func(member, value gjson.Result) bool {
		if td.exceptionSelections == nil {
			td.exceptionSelections = make(map[string]string)
		}
		td.exceptionSelections[member.String()] = value.String()
		return true
	})
}

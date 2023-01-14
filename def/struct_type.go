package def

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type structType struct {
	genericType
	isReturnedOnly bool
	members        []*structMember
}

type structMember struct {
	genericNamer
	Resolver

	pointerDepth        int
	lenSpecString       string
	lenSpecs            []string
	altLenSpec          string
	isLenForOtherMember *structMember

	fixedLengthArray bool

	typeRegistryName string
	resolvedType     TypeDefiner

	valueString   string
	resolvedValue ValueDefiner
}

func (t *structType) Category() TypeCategory { return CatStruct }

func (t *structType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return nil
	}

	t.publicName = RenameIdentifier(t.registryName)
	t.internalName = "_vk" + t.publicName

	if t.aliasTypeName != "" {
		t.resolvedAliasType = tr[t.aliasTypeName]

		if t.resolvedAliasType == nil {
			logrus.WithField("registry name", t.registryName).
				WithField("alias name", t.aliasTypeName).
				Error("alias not found in registry while resolving type")
			return nil
		} else {
			t.resolvedAliasType.Resolve(tr, vr)
			return &includeSet{
				includeTypeNames: []string{t.aliasTypeName},
			}

		}
	}

	rb := &includeSet{}

	// resolve each field of the struct
	for _, m := range t.members {
		rb.MergeWith(m.Resolve(tr, vr))
		if len(m.lenSpecs) > 0 {
			for _, n := range t.members {
				if n.RegistryName() == m.lenSpecs[0] {
					// unsafe.Pointer is a C void*. Data type is arbitrary, size
					// (and explicit cast to or from unsafe.Pointer) will have
					// to be handled by the user.
					if m.resolvedType.PublicName() != "unsafe.Pointer" && n.isLenForOtherMember == nil {
						n.isLenForOtherMember = m
					}
					break
				}
			}
		}
	}

	t.isResolved = true
	return rb
}

func (t *structType) IsIdenticalPublicAndInternal() bool {
	for _, m := range t.members {
		if !m.IsIdenticalPublicAndInternal() {
			return false
		}
	}
	return true
}

func (t *structType) TranslateToInternal(inputVar string) string {
	if t.IsIdenticalPublicAndInternal() {
		return fmt.Sprintf("%s(%s)", t.InternalName(), inputVar)
	} else {
		return fmt.Sprintf("*(%s.Vulkanize())", inputVar)
	}
}

func (t *structType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	fmt.Fprintf(w, "type %s struct {\n", t.PublicName())

	for _, m := range t.members {
		m.PrintPublicDeclaration(w)
	}

	fmt.Fprintf(w, "}\n\n")
}

func (t *structType) PrintInternalDeclaration(w io.Writer) {

	var preamble, structDecl, epilogue strings.Builder

	if t.IsIdenticalPublicAndInternal() {
		fmt.Fprintf(w, "type %s = %s\n", t.InternalName(), t.PublicName())
	} else {
		if t.isReturnedOnly {
			fmt.Fprintf(w, "// WARNING - struct %s is returned only, which is not yet handled in the binding\n", t.PublicName())
		}
		// _vk type declaration
		fmt.Fprintf(w, "type %s struct {\n", t.InternalName())
		for _, m := range t.members {
			m.PrintInternalDeclaration(w)
		}

		fmt.Fprintf(w, "}\n")
	}

	if t.isReturnedOnly {
		// Goify declaration
		fmt.Fprintf(&preamble, "func (s *%s) Goify() *%s {\n", t.InternalName(), t.PublicName())

		if t.IsIdenticalPublicAndInternal() {
			fmt.Fprintf(&structDecl, "  rval := (*%s)(s)\n", t.PublicName())
		} else {
			fmt.Fprintf(&structDecl, "  rval := &%s{\n", t.PublicName())

			for _, m := range t.members {
				m.PrintGoifyContent(&preamble, &structDecl, &epilogue)
			}
			fmt.Fprintf(&structDecl, "  }\n")
		}

		fmt.Fprintf(&epilogue, "  return rval\n")
		fmt.Fprintf(&epilogue, "}\n")

		fmt.Fprint(w, preamble.String(), structDecl.String(), epilogue.String())

	} else {

		// Vulkanize declaration
		// Set required values, like the stype
		// Expand slices to pointer and length parameters
		// Convert strings, and string arrays
		fmt.Fprintf(&preamble, "func (s *%s) Vulkanize() *%s {\n", t.PublicName(), t.InternalName())

		if t.IsIdenticalPublicAndInternal() {
			fmt.Fprintf(&structDecl, "  rval := (*%s)(s)\n", t.InternalName())
		} else {
			fmt.Fprintf(&structDecl, "  rval := &%s{\n", t.InternalName())

			for _, m := range t.members {
				m.PrintVulanizeContent(&preamble, &structDecl, &epilogue)
			}
			fmt.Fprintf(&structDecl, "  }\n")
		}

		fmt.Fprintf(&epilogue, "  return rval\n")
		fmt.Fprintf(&epilogue, "}\n")

		fmt.Fprint(w, preamble.String(), structDecl.String(), epilogue.String())
	}
}

func (m *structMember) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	m.publicName = strings.Title(RenameIdentifier(m.registryName))
	m.internalName = RenameIdentifier(m.registryName)

	// This automatically handles non-pointer types
	previousTarget := tr[m.typeRegistryName]
	for i := m.pointerDepth; i > 0; i-- {
		pt := pointerType{}
		pt.pointerDepth = i
		pt.resolvedPointsAtType = previousTarget

		if m.altLenSpec == "" {
			// If altlen is present, then the array is a fixed length per the spec.
			pt.lenSpec = m.lenSpecs[i-1]
		}

		previousTarget = &pt
	}
	m.resolvedType = previousTarget

	if m.fixedLengthArray {
		m.resolvedType = &arrayType{
			resolvedPointsAtType: tr[m.typeRegistryName],
			lenSpec:              m.lenSpecs[0],
		}
	}

	rval := includeSet{
		includeTypeNames: []string{m.typeRegistryName},
	}
	rval.MergeWith(m.resolvedType.Resolve(tr, vr))

	if m.valueString != "" {
		m.resolvedValue = vr[m.valueString]
		m.resolvedValue.Resolve(tr, vr)

		rval.includeValueNames = append(rval.includeValueNames, m.valueString)
	}

	if len(m.lenSpecs) > 0 {

	}
	return &rval
}

func (m *structMember) IsIdenticalPublicAndInternal() bool {
	return m.resolvedValue == nil &&
		m.resolvedType.IsIdenticalPublicAndInternal() &&
		m.pointerDepth == 0 &&
		m.resolvedType.Category() != CatStruct &&
		m.resolvedType.Category() != CatUnion
}

func (m *structMember) PrintPublicDeclaration(w io.Writer) {
	if m.resolvedValue != nil {
		fmt.Fprintf(w, "// %s = %s\n", m.PublicName(), m.resolvedValue.PublicName())
	} else if m.isLenForOtherMember != nil {
		fmt.Fprintf(w, "// %s\n", m.InternalName())
	} else {
		fmt.Fprintf(w, "%s %s\n", m.PublicName(), m.resolvedType.PublicName())
	}
}

func (m *structMember) PrintInternalDeclaration(w io.Writer) {
	fmt.Fprintf(w, "%s %s\n", m.InternalName(), m.resolvedType.InternalName())
}

// PrintVulkanizeContent allows struct members to write content before and after
// the returned struct declaration, as well as edit (or omit) the member's line
// in the struct.
//
// For most types, printing is as simple as assigning themselves to the field
// in the output struct, determined by checking IsIdenticalPublicAndInternal.
// This works for all primitive types (and types derived from primitives, like
// bitmasks). This also covers embedded structs that "IsIdentical..."
//
// There are a few edge cases to deal with though:
//
// 1) Fields with a fixed value, namely VkStructureType fields. We check
// resolvedValue and prinit it if that is the case.
// 2) Pointers (not array pointers) to structs. We can call Vulkanize on the
// struct and directly assign it to the output struct
// 3) Embedded structs - we can Vulkanize and dereference the embedded struct in
// the output struct declaration.
// 4) Slices of "IsIdential..." types - assign the address of the 0 element.
// 5) Slices of non-"IsIdentical..." types - build a temporary slice of the
// translated values, and then assign the 0-address as above.
// 6) Length fields - Array pointers have an associated length member in the
// struct, which needs to be set to len(slice). This is (maybe) complicated by a few
// structs having a single length field for multiple arrays.
func (m *structMember) PrintVulanizeContent(preamble, structDecl, epilogue io.Writer) {
	switch true {
	case m.resolvedValue != nil: // Edge case 1
		fmt.Fprintf(structDecl, "  %s : %s,/*c1*/\n", m.InternalName(), m.resolvedValue.PublicName())

	case m.isLenForOtherMember != nil: // Edge case 6
		fmt.Fprintf(structDecl, "  %s : %s(len(s.%s)),/*c6*/\n", m.InternalName(), m.resolvedType.PublicName(), m.isLenForOtherMember.PublicName())

	case m.resolvedType.IsIdenticalPublicAndInternal(): // Base case
		fmt.Fprintf(structDecl, "  %s : (%s)(s.%s),/*cb*/\n", m.InternalName(), m.resolvedType.InternalName(), m.PublicName())

	case m.resolvedType.Category() == CatStruct: // Edge case 3
		fmt.Fprintf(structDecl, "  %s : *(s.%s.Vulkanize()),/*c3*/\n", m.InternalName(), m.PublicName())

	// Remaining cases deal with pointers and slices
	case m.resolvedType.Category() == CatPointer:
		pt := m.resolvedType.(*pointerType)
		toBeAssigned := pt.PrintVulkanizeContent(m, preamble)
		fmt.Fprintf(structDecl, "  %s : %s,/*c rem*/\n", m.InternalName(), toBeAssigned)

	case m.resolvedType.Category() == CatArray:
		at := m.resolvedType.(*arrayType)
		toBeAssigned := at.PrintVulkanizeContent(m, preamble)

		fmt.Fprintf(structDecl, "  // %s : %s,/*c arr*/\n", m.InternalName(), toBeAssigned)

	default:
		fmt.Fprintf(structDecl, "  %s : %s,/*default*/\n", m.InternalName(), m.resolvedType.TranslateToInternal("s."+m.PublicName()))

		// fmt.Fprintf(structDecl, "%s: 0, // FELL THROUGH PRINT VULKANIZE CONTENT\n", m.InternalName())
	}
}

func (m *structMember) PrintGoifyContent(preamble, structDecl, epilogue io.Writer) {
	switch true {
	case m.resolvedValue != nil: // Edge case 1 never happens in returned strucs

	case m.isLenForOtherMember != nil: // Edge case 6 happens, but is not identified in vk.xml.
		// Example: VkPhysicalDeviceMemoryProperties has two fixed length
		// arrays, each of which has an associated lenght member to indicate how
		// many values were returned. The XML file does not link those fields
		// via "len" on the arrays. You could probably infer the information
		// because the member names are (e.g.) memoryTypeCount and memoryTypes.
		// I would like to Goify these two fields into a single slice, like we
		// do with the Enumerate commands, but will make that a future enhancement.
		// fmt.Fprintf(structDecl, "  %s : %s(len(s.%s)),/*c6*/\n", m.InternalName(), m.resolvedType.PublicName(), m.isLenForOtherMember.PublicName())
		fmt.Fprintln(structDecl, "// Unexpected 'isLenForAnotherMember'!")

	case m.resolvedType.IsIdenticalPublicAndInternal(): // Base case
		fmt.Fprintf(structDecl, "  %s : (%s)(s.%s),\n", m.PublicName(), m.resolvedType.PublicName(), m.InternalName())

	case m.resolvedType.Category() == CatStruct: // Edge case 3
		fmt.Fprintf(structDecl, "  %s : *(s.%s.Goify()),\n", m.PublicName(), m.InternalName())

	// Remaining cases deal with pointers and slices
	case m.resolvedType.Category() == CatPointer:
		// TBD if pointers ever happen in returned structs
		// pt := m.resolvedType.(*pointerType)
		// toBeAssigned := pt.PrintGoifyContent(m, preamble)
		// fmt.Fprintf(structDecl, "  %s : %s,/*c rem*/\n", m.InternalName(), toBeAssigned)

	case m.resolvedType.Category() == CatArray:
		at := m.resolvedType.(*arrayType)
		toBeAssigned := at.PrintGoifyContent(m, preamble, epilogue)

		if toBeAssigned != "" {
			fmt.Fprintf(structDecl, "  %s : %s,/*c arr*/\n", m.PublicName(), toBeAssigned)
		}

	default:
		fmt.Fprintf(structDecl, "  %s : %s,/*default*/\n", m.PublicName(), m.resolvedType.TranslateToPublic("s."+m.InternalName()))

		// fmt.Fprintf(structDecl, "%s: 0, // FELL THROUGH PRINT VULKANIZE CONTENT\n", m.InternalName())
	}
}

func ReadStructTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//type[@category='struct']") {
		s := newStructTypeFromXML(node)
		tr[s.RegistryName()] = s
	}
}

func newStructTypeFromXML(node *xmlquery.Node) *structType {
	rval := structType{}

	rval.registryName = node.SelectAttr("name")
	rval.isReturnedOnly = node.SelectAttr("returnedonly") == "true"

	for _, mNode := range xmlquery.Find(node, "member") {
		rval.members = append(rval.members, newStructMemberFromXML(mNode))
	}

	return &rval
}

// Group 1 match is numeric length, group 2 is enumeration
var rxArrayLenSpec = regexp.MustCompile(`\[(\d+)\]|<enum>(\w+)</enum>`)

func newStructMemberFromXML(node *xmlquery.Node) *structMember {
	rval := structMember{}
	rval.registryName = xmlquery.FindOne(node, "name").InnerText()

	var commentString string
	if commentNode := xmlquery.FindOne(node, "comment"); commentNode != nil {
		commentString = commentNode.InnerText()
	}
	rval.pointerDepth = strings.Count(node.InnerText(), "*") - strings.Count(commentString, "*")

	matches := rxArrayLenSpec.FindStringSubmatch(node.OutputXML(false))
	if matches != nil {
		rval.fixedLengthArray = true
		if matches[1] != "" {
			rval.lenSpecs = append(rval.lenSpecs, matches[1])
		} else if matches[2] != "" {
			rval.lenSpecs = append(rval.lenSpecs, matches[2])
		} else {
			panic("regexp unexpected matching in fixed length array")
		}
	} else {
		rval.lenSpecString = node.SelectAttr("len")
		rval.altLenSpec = strings.ReplaceAll(node.SelectAttr("altlen"), "VK_", "")

		rval.lenSpecs = strings.Split(rval.lenSpecString, ",")

	}

	// Pointers are a little odd. Generally a pointer in C becomes a slice in
	// Go, and struct members have a related length member. But in certain
	// cases, we will need handle it differently. For example, char*
	// is transformed to a string using a null byte termination.
	// (Of course this is not strictly correct, because char* implies ASCII
	// encoding, but a Go string is UTF-8.)

	rval.typeRegistryName = xmlquery.FindOne(node, "type").InnerText() // + strings.Repeat("*", rval.pointerDepth)

	rval.valueString = node.SelectAttr("values")

	return &rval

	// TODO: Some members are fixed length arrays. These are identified by []
	// around either an integer, or around <enum>VK_TYPE</enum>
}

func ReadStructExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("struct").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "comment" {
			return true
		} // Ignore comments

		entry := NewStructTypeFromJSON(key, exVal)
		tr[key.String()] = entry

		return true
	})

}

func NewStructTypeFromJSON(key, json gjson.Result) TypeDefiner {
	rval := handleType{}
	rval.registryName = key.String()
	rval.publicName = json.Get("publicName").String()
	rval.underlyingTypeName = json.Get("underlyingType").String()

	return &rval
}

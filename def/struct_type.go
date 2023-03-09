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

	forceIncludeMemberName string
	forceIncludeComment    string
}

type structMember struct {
	genericNamer
	Resolver

	pointerDepth        int
	lenSpecString       string
	lenSpecs            []string
	altLenSpec          string
	isLenForOtherMember []*structMember

	fixedLengthArray bool

	typeRegistryName string
	resolvedType     TypeDefiner

	valueString   string
	resolvedValue ValueDefiner

	forceInclude       bool
	comment            string
	noAutoValidityFlag bool
}

func (t *structType) Category() TypeCategory { return CatStruct }

func (t *structType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	t.isResolved = true // Moved here from end of function as part of issue #4 fix

	if t.publicName == "!ignore" {
		t.isResolved = true
		return NewIncludeSet()
	}

	t.publicName = RenameIdentifier(t.registryName)
	t.internalName = "_vk" + t.publicName

	if t.aliasTypeName != "" {
		t.resolvedAliasType = tr[t.aliasTypeName]

		if t.resolvedAliasType == nil {
			logrus.WithField("registry name", t.registryName).
				WithField("alias name", t.aliasTypeName).
				Error("alias not found in registry while resolving type")
			return NewIncludeSet()
		} else {
			t.resolvedAliasType.Resolve(tr, vr)
			rval := NewIncludeSet()
			rval.IncludeTypes[t.aliasTypeName] = true
			return rval
		}
	}

	rb := NewIncludeSet()

	// resolve each field of the struct
	for _, m := range t.members {
		rb.MergeWith(m.Resolve(tr, vr))
		if m.lenSpecs[0] != "" { // len(m.lenSpecs) > 0 { // len specs is always populated, if non existent then lenSpecs[0] == ""
			for _, n := range t.members {
				if n.RegistryName() == m.lenSpecs[0] {
					// unsafe.Pointer is a C void*. Data type is arbitrary, size
					// (and explicit cast to or from unsafe.Pointer) will have
					// to be handled by the user.
					if m.resolvedType.PublicName() != "unsafe.Pointer" /*&& n.isLenForOtherMember == nil*/ {
						n.isLenForOtherMember = append(n.isLenForOtherMember, m)

						// Edge case for (apparently only) VkWriteDescriptorSet...three arrays types, only one of which
						// will be populated. Flagging the len member to use the max length of the three input slices.
						// In practice, only one of the three should be populated.
						// if m.noAutoValidityFlag && len(n.isLenForOtherMember) > 1 {
						// Never mind. Just calc the max length for all multi-array structs in Vulkanize(). Most should be the same length, and Vulkan
						// validation will complain or crash if the arrays are supposed to be the same length.
						// }
					}
					break
				}
			}
		}
		m.forceInclude = t.forceIncludeMemberName == m.registryName
		if m.comment != "" {
			m.comment = m.comment + "; " + t.forceIncludeComment
		}
	}

	rb.ResolvedTypes[t.registryName] = t

	return rb
}

func (t *structType) IsIdenticalPublicAndInternal() bool {
	for _, m := range t.members {
		// part of fix for issue #4
		if asPointerType, isPointer := m.resolvedType.(*pointerType); isPointer && asPointerType.resolvedPointsAtType == t {
			continue
		}

		if !m.IsIdenticalPublicAndInternal() {
			return false
		}

	}
	return true
}

func (t *structType) TranslateToPublic(inputVar string) string {
	if t.IsIdenticalPublicAndInternal() {
		return fmt.Sprintf("%s(%s)", t.PublicName(), inputVar)
	} else {
		return fmt.Sprintf("*(%s.Goify())", inputVar)
	}
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

	if t.IsAlias() {
		fmt.Fprintf(w, "type %s = %s\n\n", t.PublicName(), t.resolvedAliasType.PublicName())
	} else {
		fmt.Fprintf(w, "type %s struct {\n", t.PublicName())

		for _, m := range t.members {
			m.PrintPublicDeclaration(w)
		}

		fmt.Fprintf(w, "}\n\n")
	}
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

	// if t.isReturnedOnly {
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

	preamble, structDecl, epilogue = strings.Builder{}, strings.Builder{}, strings.Builder{}
	// } else {

	// Vulkanize declaration
	// Set required values, like the stype
	// Expand slices to pointer and length parameters
	// Convert strings, and string arrays
	fmt.Fprintf(&preamble, "func (s *%s) Vulkanize() *%s {\n", t.PublicName(), t.InternalName())
	fmt.Fprintf(&preamble, "  if s == nil { return nil }\n")

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
	// }
}

func (m *structMember) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
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

			// It is possible to have a double pointer with a single lenSpec (see VkCuLaunchInfoNVX.pParams for an
			// example). In that case, we have to assume that the developer knows how and what they are passing in,
			// since that input will not/cannot be validated by Vulkan.
			if len(m.lenSpecs) > i-1 {
				pt.lenSpec = m.lenSpecs[i-1]
			}
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

	rval := NewIncludeSet()
	rval.IncludeTypes[m.typeRegistryName] = true

	rval.MergeWith(m.resolvedType.Resolve(tr, vr))

	if m.valueString != "" {
		m.resolvedValue = vr[m.valueString]
		rval.MergeWith(m.resolvedValue.Resolve(tr, vr))
	}

	if len(m.lenSpecs) > 0 {

	}
	return rval
}

func (m *structMember) IsIdenticalPublicAndInternal() bool {
	return m.resolvedValue == nil &&
		m.resolvedType.IsIdenticalPublicAndInternal() &&
		m.pointerDepth == 0 &&
		m.resolvedType.Category() != CatStruct &&
		m.resolvedType.Category() != CatUnion
}

func (m *structMember) PrintPublicDeclaration(w io.Writer) {

	if m.comment != "" {
		fmt.Fprintln(w, "// ", m.comment)
	}

	if m.forceInclude {
		fmt.Fprintf(w, "%s %s // Forced include via exceptions.json\n", m.PublicName(), m.resolvedType.PublicName())
	} else if m.resolvedValue != nil {
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
// 4) Slices of "IsIdentical..." types - assign the address of the 0 element.
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
		if m.forceInclude {
			fmt.Fprintf(structDecl, "  %s : s.%s,/*c6-force*/\n", m.InternalName(), m.PublicName())

		} else if len(m.isLenForOtherMember) > 1 {
			fmt.Fprintf(epilogue, "  rval.%s = 0 // c6-b\n", m.InternalName())
			for _, n := range m.isLenForOtherMember {
				fmt.Fprintf(epilogue, "  if %s(len(s.%s)) > rval.%s {\n rval.%s = %s(len(s.%s))\n }\n",
					m.resolvedType.PublicName(), n.PublicName(), m.InternalName(), m.InternalName(), m.resolvedType.PublicName(), n.PublicName())
			}

		} else {
			fmt.Fprintf(structDecl, "  %s : %s(len(s.%s)),/*c6-a*/\n", m.InternalName(), m.resolvedType.PublicName(), m.isLenForOtherMember[0].PublicName())

		}

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

	case m.resolvedType.Category() == CatUnion:
		fmt.Fprintf(structDecl, "  %s : *s.%s.Vulkanize(),/*c union*/\n", m.InternalName(), m.PublicName())

	default:
		fmt.Fprintf(structDecl, "  %s : %s,/*default*/\n", m.InternalName(), m.resolvedType.TranslateToInternal("s."+m.PublicName()))

		// fmt.Fprintf(structDecl, "%s: 0, // FELL THROUGH PRINT VULKANIZE CONTENT\n", m.InternalName())
	}
}

func (m *structMember) PrintGoifyContent(preamble, structDecl, epilogue io.Writer) {
	switch true {
	case m.resolvedValue != nil: // Edge case 1 never happens in returned strucs

	case m.resolvedType.Category() == CatUnion:
		fmt.Fprintf(structDecl, "  // Can't Goify union member %s\n", m.InternalName())

	case m.isLenForOtherMember != nil: // Edge case 6 happens, but is not identified in vk.xml.
		// Example: VkPhysicalDeviceMemoryProperties has two fixed length
		// arrays, each of which has an associated length member to indicate how
		// many values were returned. The XML file does not link those fields
		// via "len" on the arrays. You could probably infer the information
		// because the member names are (e.g.) memoryTypeCount and memoryTypes.
		// I would like to Goify these two fields into a single slice, like we
		// do with the Enumerate commands, but will make that a future enhancement.
		// fmt.Fprintf(structDecl, "  %s : %s(len(s.%s)),/*c6*/\n", m.InternalName(), m.resolvedType.PublicName(), m.isLenForOtherMember.PublicName())
		fmt.Fprintln(structDecl, "// Unexpected 'isLenForAnotherMember'!")

	case m.resolvedType.IsIdenticalPublicAndInternal(): // Base case
		fmt.Fprintf(structDecl, "  %s : (%s)(s.%s),\n", m.PublicName(), m.resolvedType.PublicName(), m.InternalName())

	case m.resolvedType.Category() == CatUnion: // Edge case 3a
		// This will almost surely fail
		fallthrough
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

	if commentNode := xmlquery.FindOne(node, "comment"); commentNode != nil {
		rval.comment = commentNode.InnerText()
	}
	rval.pointerDepth = strings.Count(node.InnerText(), "*") - strings.Count(rval.comment, "*")

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

	rval.noAutoValidityFlag = node.SelectAttr("noautovalidity") == "true"

	// Pointers are a little odd. Generally a pointer in C becomes a slice in
	// Go, and struct members have a related length member. But in certain
	// cases, we will need handle it differently. For example, char*
	// is transformed to a string using a null byte termination.
	// (Of course this is not strictly correct, because char* implies ASCII
	// encoding, but a Go string is UTF-8.)

	rval.typeRegistryName = xmlquery.FindOne(node, "type").InnerText() // + strings.Repeat("*", rval.pointerDepth)
	if rval.typeRegistryName == "void" && rval.pointerDepth == 1 {
		// special case for void* struct members...exceptions.json maps the member to unsafe.Pointer instead of byte*
		rval.typeRegistryName = "void*"
		rval.pointerDepth = 0
	}

	rval.valueString = node.SelectAttr("values")

	return &rval

	// TODO: Some members are fixed length arrays. These are identified by []
	// around either an integer, or around <enum>VK_TYPE</enum>
}

func ReadStructExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("struct").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
			return true
		} // Ignore comments

		var existing *structType
		if tmp, found := tr[key.String()]; found {
			existing = tmp.(*structType)
		}

		entry := NewOrUpdateStructTypeFromJSON(key, exVal, existing)
		tr[key.String()] = entry

		return true
	})

}

func NewOrUpdateStructTypeFromJSON(key, json gjson.Result, existing *structType) TypeDefiner {
	var rval *structType = existing
	if existing == nil {
		rval = &structType{}
	}

	rval.registryName = key.String()

	if json.Get("publicName").String() != "" {
		rval.publicName = json.Get("publicName").String()
	}

	rval.forceIncludeMemberName = json.Get("forceIncludeMember").String()
	rval.forceIncludeComment = json.Get("forceIncludeComment").String()

	return rval
}

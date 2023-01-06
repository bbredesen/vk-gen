package def

import (
	"fmt"
	"io"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type structType struct {
	genericType
	members []*structMember
}

type structMember struct {
	genericNamer
	Resolver

	pointerDepth  int
	lenSpecString string
	lenSpecs      []string
	altLenSpec    string

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

	t.publicName = renameIdentifier(t.registryName)
	t.internalName = "_vk" + t.publicName

	if t.aliasRegistryName != "" {
		t.resolvedAliasType = tr[t.aliasRegistryName]

		if t.resolvedAliasType == nil {
			logrus.WithField("registry name", t.registryName).
				WithField("alias name", t.aliasRegistryName).
				Error("alias not found in registry while resolving type")
			return nil
		} else {
			t.resolvedAliasType.Resolve(tr, vr)
			return &includeSet{
				includeTypeNames: []string{t.aliasRegistryName},
			}

		}
	}

	rb := &includeSet{}

	// resolve each field of the struct
	for _, m := range t.members {
		rb.mergeWith(m.Resolve(tr, vr))
	}

	t.isResolved = true
	return rb
}

func (t *structType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	fmt.Fprintf(w, "type %s struct {\n", t.PublicName())

	for _, m := range t.members {
		m.PrintPublicDeclaration(w)
	}

	fmt.Fprintf(w, "}\n\n")
}

func (m *structMember) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	m.publicName = renameIdentifier(m.registryName)
	m.internalName = m.registryName

	if m.pointerDepth > 0 {
		pt := pointerType{}
		pt.resolvedPointsAtType = tr[m.typeRegistryName]

		m.resolvedType = &pt
	} else {
		m.resolvedType = tr[m.typeRegistryName]
	}

	rval := includeSet{
		includeTypeNames: []string{m.typeRegistryName},
	}
	rval.mergeWith(m.resolvedType.Resolve(tr, vr))

	if m.valueString != "" {
		m.resolvedValue = vr[m.valueString]
		m.resolvedValue.Resolve(tr, vr)

		rval.includeValueNames = append(rval.includeValueNames, m.valueString)
	}
	return &rval
}

func (m *structMember) PrintPublicDeclaration(w io.Writer) {
	if m.resolvedValue != nil {
		fmt.Fprintf(w, "// %s = %s\n", m.PublicName(), m.resolvedValue.PublicName())
		return
	}

	fmt.Fprintf(w, "%s %s\n", m.PublicName(), m.resolvedType.PublicName())
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

	for _, mNode := range xmlquery.Find(node, "member") {
		rval.members = append(rval.members, newStructMemberFromXML(mNode))
	}

	return &rval
}

func newStructMemberFromXML(node *xmlquery.Node) *structMember {
	rval := structMember{}

	var commentString string
	if commentNode := xmlquery.FindOne(node, "comment"); commentNode != nil {
		commentString = commentNode.InnerText()
	}
	rval.pointerDepth = strings.Count(node.InnerText(), "*") - strings.Count(commentString, "*")

	// rval.lenSpecString = node.SelectAttr() // TODO

	rval.registryName = xmlquery.FindOne(node, "name").InnerText()
	rval.typeRegistryName = xmlquery.FindOne(node, "type").InnerText()

	rval.valueString = node.SelectAttr("values")

	return &rval
}

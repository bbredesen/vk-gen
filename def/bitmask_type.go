package def

import (
	"fmt"
	"io"
	"sort"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type bitmaskType struct {
	internalType

	requiresTypeName     string
	resolvedRequiresType TypeDefiner

	bitwidth string
}

func (t *bitmaskType) Category() TypeCategory             { return CatBitmask }
func (t *bitmaskType) IsIdenticalPublicAndInternal() bool { return true }

func (t *bitmaskType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return nil
	}

	rval := t.internalType.Resolve(tr, vr)

	if t.requiresTypeName != "" {
		// requiresType is the enum type; enum type needs to be defined as
		// equivalent to this type in the output
		t.resolvedRequiresType = tr[t.requiresTypeName]
		rval.MergeWith(t.resolvedRequiresType.Resolve(tr, vr))

		// Force set the enum's underlying type to be this bitmaskType
		(t.resolvedRequiresType).(*enumType).underlyingTypeName = t.RegistryName()
		(t.resolvedRequiresType).(*enumType).underlyingType = t

		// rval.includeTypeNames = append(rval.includeTypeNames, t.requiresTypeName)
		// rval.MergeWith(t.resolvedRequiresType.Resolve(tr, vr))
	}

	t.isResolved = true
	return rval
}

func (t *bitmaskType) PrintPublicDeclaration(w io.Writer) {
	t.internalType.PrintPublicDeclaration(w)

	sort.Sort(byValue(t.values))

	if len(t.values) > 0 {
		fmt.Fprint(w, "const (\n")
		for i, v := range t.values {
			v.PrintPublicDeclaration(w, i == 0) // || !v.IsAlias())
		}
		fmt.Fprint(w, ")\n\n")
	}
}

func ReadBitmaskTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//type[@category='bitmask']") {
		newType := NewBitmaskTypeFromXML(node)
		if tr[newType.RegistryName()] != nil {
			logrus.WithField("registry name", newType.RegistryName()).Warn("Overwriting bitmask type in registry")
		}
		tr[newType.RegistryName()] = newType
	}
}

func NewBitmaskTypeFromXML(node *xmlquery.Node) TypeDefiner {
	rval := bitmaskType{}

	if alias := node.SelectAttr("alias"); alias != "" {
		rval.aliasTypeName = alias
		rval.registryName = node.SelectAttr("name")
	} else {
		rval.registryName = xmlquery.FindOne(node, "name").InnerText()
		rval.underlyingTypeName = xmlquery.FindOne(node, "type").InnerText()
		rval.requiresTypeName = node.SelectAttr("requires")
	}

	return &rval
}

package def

import (
	"fmt"
	"io"
	"sort"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type bitmaskType struct {
	definedType
	requiresTypeName     string
	resolvedRequiresType TypeDefiner
}

func (t *bitmaskType) Category() TypeCategory { return CatBitmask }

func (t *bitmaskType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return nil
	}

	rval := t.definedType.Resolve(tr, vr)

	if t.requiresTypeName != "" {
		t.resolvedRequiresType = tr[t.requiresTypeName]
		t.resolvedRequiresType.SetAliasType(t)

		rval.includeTypeNames = append(rval.includeTypeNames, t.requiresTypeName)
		rval.mergeWith(t.resolvedRequiresType.Resolve(tr, vr))
	}

	return rval
}

func (t *bitmaskType) PrintPublicDeclaration(w io.Writer) {
	// if t.ValuesAreIncludedElsewhere() {
	// 	return
	// }

	// fmt.Fprintf(w, "// %s: See https://www.khronos.org/registry/vulkan/specs/1.3-extensions/man/html/%s.html\n",
	// 	t.PublicName(), t.RegistryName(),
	// )
	t.PrintDocLink(w)

	if t.comment != "" {
		fmt.Fprintln(w, "// ", t.comment)
	}

	if t.IsAlias() {
		fmt.Fprintf(w, "type %s = %s\n", t.PublicName(), t.resolvedAliasType.PublicName())
	} else {
		fmt.Fprintf(w, "type %s %s\n", t.PublicName(), t.resolvedUnderlyingType.PublicName())
	}

	sort.Sort(byValue(t.values))

	if len(t.values) > 0 {
		fmt.Fprint(w, "const (\n")
		for _, v := range t.values {
			v.PrintPublicDeclaration(w, !v.IsAlias())
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

		// ReadBitmaskValuesFromXML(doc, newType, tr, vr)
	}
}

func NewBitmaskTypeFromXML(node *xmlquery.Node) TypeDefiner {
	rval := bitmaskType{}

	if alias := node.SelectAttr("alias"); alias != "" {
		rval.aliasRegistryName = alias
		rval.registryName = node.SelectAttr("name")
	} else {
		rval.registryName = xmlquery.FindOne(node, "name").InnerText()
		rval.underlyingTypeName = xmlquery.FindOne(node, "type").InnerText()
		rval.requiresTypeName = node.SelectAttr("requires")
	}

	rval.publicName = renameIdentifier(rval.registryName)

	return &rval
}

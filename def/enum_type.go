package def

import (
	"fmt"
	"io"
	"sort"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type enumType struct {
	definedType
}

func (t *enumType) Category() TypeCategory { return CatEnum }

func (t *enumType) PrintPublicDeclaration(w io.Writer) {
	// if t.ValuesAreIncludedElsewhere() {
	// 	return
	// }
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

func ReadEnumTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//type[@category='enum']") {
		newType := NewEnumTypeFromXML(node)
		if tr[newType.RegistryName()] != nil {
			logrus.WithField("registry name", newType.RegistryName()).Warn("Overwriting enum type in registry")
		}
		tr[newType.RegistryName()] = newType

		ReadEnumValuesFromXML(doc, newType, tr, vr)
	}
}

func NewEnumTypeFromXML(node *xmlquery.Node) TypeDefiner {
	rval := enumType{}

	if alias := node.SelectAttr("alias"); alias != "" {
		rval.aliasRegistryName = alias
		rval.registryName = node.SelectAttr("name")
	} else {
		rval.registryName = node.SelectAttr("name")
		rval.underlyingTypeName = "int32_t"
	}

	rval.publicName = renameIdentifier(rval.registryName)

	return &rval
}

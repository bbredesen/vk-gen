package def

import (
	"fmt"
	"io"
	"sort"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type enumType struct {
	internalType

	requiresTypeName     string
	resolvedRequiresType TypeDefiner
}

func (t *enumType) Category() TypeCategory { return CatEnum }

func (t *enumType) IsIdenticalPublicAndInternal() bool { return true }

func (t *enumType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return nil
	}

	rval := t.internalType.Resolve(tr, vr)

	// if t.requiresTypeName != "" {
	// 	t.resolvedRequiresType = tr[t.requiresTypeName]
	// 	t.resolvedRequiresType.SetAliasType(t)
	// 	rval.MergeWith(t.resolvedRequiresType.Resolve(tr, vr))

	// 	rval.includeTypeNames = append(rval.includeTypeNames, t.requiresTypeName)
	// }

	t.isResolved = true
	return rval
}

func (t *enumType) PrintPublicDeclaration(w io.Writer) {
	if t.underlyingType != nil && t.underlyingType.Category() == CatBitmask {
		fmt.Fprintf(w, "type %s = %s\n", t.PublicName(), t.underlyingType.PublicName())
	} else {
		t.internalType.PrintPublicDeclaration(w)
	}

	sort.Sort(byValue(t.values))

	if len(t.values) > 0 {
		fmt.Fprint(w, "const (\n")
		for i, v := range t.values {
			v.PrintPublicDeclaration(w, i == 0) // || !v.IsAlias())
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
		rval.aliasTypeName = alias
		rval.registryName = node.SelectAttr("name")
	} else {
		rval.registryName = node.SelectAttr("name")
		rval.underlyingTypeName = "int32_t"
	}

	return &rval
}

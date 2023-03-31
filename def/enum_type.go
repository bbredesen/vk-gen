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
	isBitmaskType        bool
}

func (t *enumType) Category() TypeCategory { return CatEnum }

func (t *enumType) IsIdenticalPublicAndInternal() bool { return true }

func (t *enumType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	rval := t.internalType.Resolve(tr, vr)
	rval.ResolvedTypes[t.registryName] = t

	if t.requiresTypeName != "" {
		rval.MergeWith(tr[t.requiresTypeName].Resolve(tr, vr))
	}

	t.isResolved = true
	return rval
}

func (t *enumType) PrintPublicDeclaration(w io.Writer) {
	if t.isBitmaskType {
		fmt.Fprintf(w, "type %s = %s\n", t.PublicName(), t.underlyingType.PublicName())
	} else {
		t.internalType.PrintPublicDeclaration(w)
	}

	sort.Sort(ByValue(t.values))

	if t.RegistryName() == "VkResult" {
		fmt.Fprintf(w, "// Command completed successfully\nvar SUCCESS error = nil\n")
	}

	if len(t.values) > 0 {
		fmt.Fprint(w, "const (\n")
		for _, v := range t.values {
			v.PrintPublicDeclaration(w)
		}
		fmt.Fprint(w, ")\n\n")
	}
}

func ReadEnumTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry, api string) {
	queryString := fmt.Sprintf("//types/type[@category='enum' and (@api='%s' or not(@api))]", api)

	for _, node := range xmlquery.Find(doc, queryString) {
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

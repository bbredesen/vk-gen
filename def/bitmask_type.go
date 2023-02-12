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

	valuesTypeName string
}

func (t *bitmaskType) Category() TypeCategory             { return CatBitmask }
func (t *bitmaskType) IsIdenticalPublicAndInternal() bool { return true }

func (t *bitmaskType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	rval := NewIncludeSet()

	rval.MergeWith(t.internalType.Resolve(tr, vr))

	rval.IncludeTypes[t.registryName] = true
	rval.ResolvedTypes[t.registryName] = t

	t.isResolved = true

	return rval
}

func (t *bitmaskType) PrintPublicDeclaration(w io.Writer) {
	t.internalType.PrintPublicDeclaration(w)

	sort.Sort(byValue(t.values))

	if len(t.values) > 0 {
		fmt.Fprint(w, "const (\n")
		for _, v := range t.values {
			v.PrintPublicDeclaration(w) // || !v.IsAlias())
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

		// Attach bitmask to the associated enum. CatEnum must be read from the file first!
		if newType.valuesTypeName != "" {
			// requiresType is the enum type; enum type needs to be defined as
			// equivalent to this type in the output
			// newType.resolvedRequiresType = tr[newType.valuesTypeName]

			if r, ok := (tr[newType.valuesTypeName]).(*enumType); ok {
				// Force set the enum's underlying type to be this bitmaskType
				r.underlyingTypeName = newType.registryName
			} else {
				panic("Bitmask requires non-enum as underlying type!")
			}
		}

		tr[newType.RegistryName()] = newType
	}
}

func NewBitmaskTypeFromXML(node *xmlquery.Node) *bitmaskType {
	rval := bitmaskType{}

	if alias := node.SelectAttr("alias"); alias != "" {
		rval.aliasTypeName = alias
		rval.registryName = node.SelectAttr("name")
	} else {
		rval.registryName = xmlquery.FindOne(node, "name").InnerText()
		rval.underlyingTypeName = xmlquery.FindOne(node, "type").InnerText()

		// VkFlags64 uses a bitvalues attribute, but VkFlags (32-bit) uses a requires attribute. This is documented in
		// the API Registry document, but not explained why.
		rval.valuesTypeName = node.SelectAttr("requires")
		if bits := node.SelectAttr("bitvalues"); bits != "" {
			rval.valuesTypeName = bits
		}
	}

	return &rval
}

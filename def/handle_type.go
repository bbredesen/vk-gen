package def

import (
	"fmt"
	"io"
	"sort"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type handleType struct {
	internalType
}

func (t *handleType) Category() TypeCategory { return CatHandle }

func (t *handleType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	rval := t.internalType.Resolve(tr, vr)

	rval.ResolvedTypes[t.registryName] = t

	t.isResolved = true
	return rval
}

func (t *handleType) PrintPublicDeclaration(w io.Writer) {
	t.internalType.PrintPublicDeclaration(w)

	sort.Sort(byValue(t.values))

	if len(t.values) > 0 {
		fmt.Fprint(w, "const (\n")
		for _, v := range t.values {
			v.PrintPublicDeclaration(w)
		}
		fmt.Fprint(w, ")\n\n")
	}
}

func ReadHandleTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, _ ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//type[@category='handle']") {
		newType := NewHandleTypeFromXML(node)
		if tr[newType.RegistryName()] != nil {
			logrus.WithField("registry name", newType.RegistryName()).Warn("Overwriting handle type in registry")
		}
		tr[newType.RegistryName()] = newType
	}
}

func NewHandleTypeFromXML(node *xmlquery.Node) TypeDefiner {
	rval := handleType{}

	if alias := node.SelectAttr("alias"); alias != "" {
		rval.aliasTypeName = alias
		rval.registryName = node.SelectAttr("name")
	} else {
		rval.registryName = xmlquery.FindOne(node, "name").InnerText()
		rval.underlyingTypeName = xmlquery.FindOne(node, "type").InnerText()
	}

	rval.publicName = RenameIdentifier(rval.registryName)

	return &rval
}

func ReadHandleExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("handle").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "comment" {
			return true
		} // Ignore comments

		entry := NewHandleTypeFromJSON(key, exVal)
		tr[key.String()] = entry

		exVal.Get("constants").ForEach(func(ck, cv gjson.Result) bool {
			newVal := enumValue{}
			newVal.registryName = ck.String()
			newVal.valueString = cv.String()
			newVal.underlyingTypeName = key.String()

			// newVal := NewEnumValue(ck.String(), cv.String(), key.String())

			vr[newVal.RegistryName()] = &newVal
			return true
		})

		return true
	})

}

func NewHandleTypeFromJSON(key, json gjson.Result) TypeDefiner {
	rval := handleType{}
	rval.registryName = key.String()
	rval.publicName = json.Get("publicName").String()
	rval.underlyingTypeName = json.Get("underlyingType").String()

	return &rval
}

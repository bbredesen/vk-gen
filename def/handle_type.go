package def

import (
	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type handleType struct {
	definedType
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
		rval.aliasRegistryName = alias
		rval.registryName = node.SelectAttr("name")
	} else {
		rval.registryName = xmlquery.FindOne(node, "name").InnerText()
		rval.underlyingTypeName = xmlquery.FindOne(node, "type").InnerText()
	}

	rval.publicName = renameIdentifier(rval.registryName)

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
			newVal := NewConstantValue(ck.String(), cv.String(), key.String())

			vr[newVal.RegistryName()] = newVal
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

func (t *handleType) Category() TypeCategory { return CatHandle }

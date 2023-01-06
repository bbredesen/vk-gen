package def

import (
	"fmt"
	"io"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type baseType struct {
	definedType
}

func ReadBaseTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, _ ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//type[@category='basetype']") {
		newType := NewBaseTypeFromXML(node)
		if tr[newType.RegistryName()] != nil {
			logrus.WithField("registry name", newType.RegistryName()).Warn("Overwriting base type in registry")
		}
		tr[newType.RegistryName()] = newType
	}
}

func NewBaseTypeFromXML(node *xmlquery.Node) TypeDefiner {
	rval := baseType{}
	rval.registryName = xmlquery.FindOne(node, "name").InnerText()
	rval.publicName = renameIdentifier(rval.registryName)

	typeNode := xmlquery.FindOne(node, "type")
	if typeNode == nil {
		rval.underlyingTypeName = "!empty_struct"
	} else {
		rval.underlyingTypeName = typeNode.InnerText()
	}

	return &rval
}

func ReadBaseTypeExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("basetype").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "comment" {
			return true
		} // Ignore comments

		entry := NewBaseTypeFromJSON(key, exVal)
		tr[key.String()] = entry

		exVal.Get("constants").ForEach(func(ck, cv gjson.Result) bool {
			newVal := NewConstantValue(ck.String(), cv.String(), key.String())

			vr[newVal.RegistryName()] = newVal
			return true
		})

		return true
	})

}
func NewBaseTypeFromJSON(key, json gjson.Result) TypeDefiner {
	rval := baseType{}

	rval.registryName = key.String()
	rval.publicName = renameIdentifier(rval.registryName)
	rval.underlyingTypeName = json.Get("underlyingTypeName").String()
	rval.aliasRegistryName = json.Get("aliasName").String()

	return &rval

}

func (t *baseType) PrintTranslateToInternal(w io.Writer, inputVar, outputVar string) {
	if t.registryName == "VkBool32" {
		// special case
		fmt.Fprintf(w, "if %s {\n  %s = TRUE\n} else {\n  %s = FALSE\n}\n", inputVar, outputVar, outputVar)
	} else {
		t.definedType.PrintTranslateToInternal(w, inputVar, outputVar)
	}
}

func (t *baseType) PrintTranslateToPublic(w io.Writer, inputVar, outputVar string) {
	if t.registryName == "VkBool32" {
		// special case
		fmt.Fprintf(w, "%s = (%s == TRUE)\n", outputVar, inputVar)
	} else {
		t.definedType.PrintTranslateToInternal(w, inputVar, outputVar)
	}
}

func (t *baseType) Category() TypeCategory { return CatBasetype }

package def

import (
	"github.com/antchfx/xmlquery"
)

type constantValue struct {
	genericValue
}

func ReadAPIConstantsFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//enums[@name='API Constants']/enum") {
		var cv *constantValue
		if alias := node.SelectAttr("alias"); alias != "" {
			cv = NewConstantAliasValue(node.SelectAttr("name"), alias)
		} else {
			cv = NewConstantValue(node.SelectAttr("name"), node.SelectAttr("value"), node.SelectAttr("type"))
		}

		vr[cv.RegistryName()] = cv
	}
}

func NewConstantValue(name, value, typ string) *constantValue {
	rval := constantValue{}
	rval.registryName = name
	rval.valueString = value
	rval.underlyingTypeName = typ
	return &rval
}

func NewConstantAliasValue(name, alias string) *constantValue {
	rval := constantValue{}
	rval.registryName = name
	rval.aliasValueName = alias
	return &rval
}

// func (v *constantValue) PrintPublicDeclaration(w io.Writer, withExplicitType bool) {
// 	if withExplicitType {
// 		fmt.Fprintf(w, "%s %s = %s\n", v.PublicName(), v.resolvedType.PublicName(), v.valueString)
// 	} else {
// 		fmt.Fprintf(w, "%s = %s\n", v.PublicName(), v.valueString)
// 	}
// }

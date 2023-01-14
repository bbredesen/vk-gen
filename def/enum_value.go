package def

import (
	"fmt"
	"io"
	"strconv"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type enumValue struct {
	genericValue

	extNumber int
	offset    int
	direction int

	bitposString string

	comment string

	sourceOrder int
}

func (v *enumValue) ValueString() string {
	if v.IsAlias() {
		return v.resolvedAliasValue.PublicName()
	} else {
		return v.valueString
	}
}

func (v *enumValue) Resolve(tr TypeRegistry, vr ValueRegistry) {
	if v.isResolved {
		return
	}

	if v.IsAlias() {
		v.resolvedAliasValue = vr[v.aliasValueName]
		v.resolvedAliasValue.Resolve(tr, vr)
		v.valueString = RenameIdentifier(v.ValueString())

		v.resolvedType = v.resolvedAliasValue.ResolvedType()
		v.resolvedType.Resolve(tr, vr)
	} else {
		v.resolvedType = tr[v.underlyingTypeName]
		v.resolvedType.Resolve(tr, vr)
	}

	v.resolvedType.PushValue(v)

	v.isResolved = true
}

func (v *enumValue) PrintPublicDeclaration(w io.Writer, withExplicitType bool) {
	if v.comment != "" {
		fmt.Fprintf(w, "// %s\n", v.comment)
	}

	if withExplicitType {
		fmt.Fprintf(w, "%s %s = %s\n", v.PublicName(), v.resolvedType.PublicName(), v.ValueString())
	} else {
		fmt.Fprintf(w, "%s = %s\n", v.PublicName(), v.ValueString())
	}
}

func ReadApiConstantsFromXML(doc *xmlquery.Node, externalType TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	for _, node := range xmlquery.Find(doc, fmt.Sprintf("//enums[@name='API Constants']/enum[@type='%s']", externalType.RegistryName())) {
		valDef := NewEnumValueFromXML(externalType, node)
		vr[valDef.RegistryName()] = valDef
		externalType.PushValue(valDef)
	}
}

func ReadEnumValuesFromXML(doc *xmlquery.Node, td TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	groupSearchNodes := xmlquery.Find(doc, fmt.Sprintf("//enums[@name='%s']", td.RegistryName()))

	for _, groupNode := range groupSearchNodes {
		coreVals := append(xmlquery.Find(groupNode, "/enum"))
		extVals := xmlquery.Find(doc, fmt.Sprintf("//require/enum[@extends='%s']", td.RegistryName()))

		switch groupNode.SelectAttr("type") {
		case "bitmask":
			for _, enumNode := range coreVals {
				valDef := NewBitmaskValueFromXML(td, enumNode)
				valDef.isCore = true
				vr[valDef.RegistryName()] = valDef
			}
			for _, enumNode := range extVals {
				valDef := NewBitmaskValueFromXML(td, enumNode)
				valDef.isCore = false
				vr[valDef.RegistryName()] = valDef
			}
		case "enum":
			for _, enumNode := range coreVals {
				valDef := NewEnumValueFromXML(td, enumNode)
				valDef.isCore = true
				vr[valDef.RegistryName()] = valDef
			}
			for _, enumNode := range extVals {
				valDef := NewEnumValueFromXML(td, enumNode)
				valDef.isCore = false
				vr[valDef.RegistryName()] = valDef
			}
		}
	}
}

func NewEnumValueFromXML(td TypeDefiner, elt *xmlquery.Node) *enumValue {
	rval := enumValue{}

	alias := elt.SelectAttr("alias")
	if alias == "" {
		rval.registryName = elt.SelectAttr("name")
		rval.valueString = elt.SelectAttr("value")
	} else {
		rval.registryName = elt.SelectAttr("name")
		rval.aliasValueName = alias
	}
	rval.comment = elt.SelectAttr("comment")

	if rval.underlyingTypeName = elt.SelectAttr("extends"); rval.underlyingTypeName != "" {
		var err error

		if offsetStr := elt.SelectAttr("offset"); offsetStr != "" {
			if rval.offset, err = strconv.Atoi(offsetStr); err != nil {
				logrus.WithField("registry name", rval.registryName).
					WithField("offset", offsetStr).
					WithError(err).
					Error("could not convert enum offset string")
				return &rval
			}
		}
		if elt.SelectAttr("dir") == "-" {
			rval.direction = -1
		} else {
			rval.direction = 1
		}

		// Only applies when an extension is promoted to core, otherwise the Extensioner needs to add the extention number to the value
		extNumStr := elt.SelectAttr("extnumber")
		if extNumStr != "" {
			if rval.extNumber, err = strconv.Atoi(extNumStr); err != nil {
				logrus.WithField("registry name", rval.registryName).
					WithField("offset", extNumStr).
					WithError(err).
					Error("could not convert enum extension number")
				return &rval
			}
		}
	} else {
		rval.underlyingTypeName = td.RegistryName()
	}

	return &rval
}

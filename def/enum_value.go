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
	} else if v.bitposString != "" {
		return fmt.Sprintf("1 << %s", v.bitposString)
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
		v.valueString = renameIdentifier(v.ValueString())

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

func ReadEnumValuesFromXML(doc *xmlquery.Node, td TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	for _, enumNode := range xmlquery.Find(doc, fmt.Sprintf("//enums[@name='%s']/enum", td.RegistryName())) {
		valDef := NewEnumValueFromXML(td.RegistryName(), enumNode)
		vr[valDef.RegistryName()] = valDef
	}
}

func NewEnumValueFromXML(enumName string, elt *xmlquery.Node) ValueDefiner {
	rval := enumValue{}

	alias := elt.SelectAttr("alias")
	if alias == "" {
		rval.registryName = elt.SelectAttr("name")
		rval.valueString = elt.SelectAttr("value")
		rval.bitposString = elt.SelectAttr("bitpos")
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
		rval.underlyingTypeName = enumName
	}

	return &rval
}

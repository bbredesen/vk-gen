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

	bitposString string

	comment string

	sourceOrder int
}

func (v *enumValue) ValueString() string {
	if v.IsAlias() {
		return v.resolvedAliasValue.PublicName()
	} else {
		if v.extNumber != 0 {
			tmp := (1000000000 + (v.extNumber-1)*1000 + v.offset) * v.direction
			return strconv.Itoa(tmp)
		}
		return v.valueString
	}
}

func (v *enumValue) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if v.isResolved {
		return NewIncludeSet()
	}
	var rval *IncludeSet

	if v.IsAlias() {
		v.resolvedAliasValue = vr[v.aliasValueName]
		v.resolvedAliasValue.Resolve(tr, vr)
		v.valueString = RenameIdentifier(v.ValueString())

		v.resolvedType = v.resolvedAliasValue.ResolvedType()
		rval = v.resolvedType.Resolve(tr, vr)
	} else {
		v.resolvedType = tr[v.underlyingTypeName]
		rval = v.resolvedType.Resolve(tr, vr)
	}

	rval.IncludeValues[v.registryName] = true
	rval.ResolvedValues[v.registryName] = v

	v.isResolved = true
	return rval
}

func (v *enumValue) PrintPublicDeclaration(w io.Writer) {
	if v.comment != "" {
		fmt.Fprintf(w, "// %s\n", v.comment)
	}

	fmt.Fprintf(w, "%s %s = %s\n", v.PublicName(), v.resolvedType.PublicName(), v.ValueString())

}

func ReadApiConstantsFromXML(doc *xmlquery.Node, externalType TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	var selector string
	if externalType == nil {
		selector = "//enums[@name='API Constants']/enum[not(@type)]"
	} else {
		selector = fmt.Sprintf("//enums[@name='API Constants']/enum[@type='%s']", externalType.RegistryName())
	}
	for _, node := range xmlquery.Find(doc, selector) {
		valDef := NewEnumValueFromXML(externalType, node)
		valDef.isCore = true
		vr[valDef.RegistryName()] = valDef
		// externalType.PushValue(valDef)
	}
}

func ReadEnumValuesFromXML(doc *xmlquery.Node, td TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	groupSearchNodes := xmlquery.Find(doc, fmt.Sprintf("//enums[@name='%s']", td.RegistryName()))

	for _, groupNode := range groupSearchNodes {
		coreVals := append(xmlquery.Find(groupNode, "/enum"))
		extVals := xmlquery.Find(doc, fmt.Sprintf("//require/enum[@extends='%s']", td.RegistryName()))

		switch groupNode.SelectAttr("type") {
		case "bitmask":
			td.(*enumType).isBitmaskType = true
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

				// Some enums from extensions are double-defined (when promoted to core maybe?) See
				// VK_STRUCTURE_TYPE_DEVICE_GROUP_PRESENT_INFO_KHR
				// To handle this, we're reading the node and then merging with any val def already in the map

				// valDef.isCore = valDef.extNumber != 0

				if existing, found := vr[valDef.RegistryName()]; found {
					existingAsEnum := existing.(*enumValue)
					if valDef.extNumber == 0 {
						valDef.extNumber = existingAsEnum.extNumber
					}
					if valDef.offset == 0 {
						valDef.offset = existingAsEnum.offset
					}
					if valDef.direction == 0 {
						valDef.direction = existingAsEnum.direction
					}
				}
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
	} else if td != nil {
		rval.underlyingTypeName = td.RegistryName()
	}

	return &rval
}

type extenValue struct {
	enumValue
}

func (v *extenValue) Category() TypeCategory { return CatExten }
func (v *extenValue) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if v.isResolved {
		return NewIncludeSet()
	}
	v.isResolved = true
	rval := NewIncludeSet()
	rval.ResolvedValues[v.registryName] = v
	return rval
}

func (v *extenValue) PrintPublicDeclaration(w io.Writer) {
	if v.comment != "" {
		fmt.Fprintf(w, "// %s\n", v.comment)
	}

	// Ignore explicit type, these values are untyped in the spec and the inferred type in Go is fine for our purpose
	fmt.Fprintf(w, "%s = %s\n", v.PublicName(), v.ValueString())
}

func (v *extenValue) ValueString() string {
	return v.valueString
}

func NewUntypedEnumValueFromXML(elt *xmlquery.Node) *extenValue {
	rval := extenValue{}

	alias := elt.SelectAttr("alias") // I don't think there are any alias entries for this category?
	if alias == "" {
		rval.registryName = elt.SelectAttr("name")
		rval.valueString = elt.SelectAttr("value")
	} else {
		rval.registryName = elt.SelectAttr("name")
		rval.aliasValueName = alias
	}
	rval.comment = elt.SelectAttr("comment")

	// rval.underlyingTypeName = "!none"

	return &rval
}

const test = "ABC"

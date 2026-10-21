package def

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type enumValue struct {
	genericValue

	comment string

	sourceOrder int
}

func (v *enumValue) ValueString() string {
	if v.IsAlias() {
		return v.resolvedAliasValue.PublicName()
	} else {
		// An explicit value (e.g. an extension's *_NONE_KHR = 0) takes precedence over the extension offset formula
		if v.valueString == "" && v.extNumber != 0 {
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
	var rval *IncludeSet = NewIncludeSet()

	if v.IsAlias() {
		v.resolvedAliasValue = vr[v.aliasValueName]
		rval.MergeWith(v.resolvedAliasValue.Resolve(tr, vr))
		v.valueString = RenameIdentifier(v.ValueString())

		v.resolvedType = v.resolvedAliasValue.ResolvedType()
		rval.MergeWith(v.resolvedType.Resolve(tr, vr))
	} else {
		v.resolvedType = tr[v.underlyingTypeName]
		rval.MergeWith(v.resolvedType.Resolve(tr, vr))
	}

	rval.IncludeValues[v.registryName] = true
	rval.ResolvedValues[v.registryName] = v

	v.isResolved = true
	return rval
}

func (v *enumValue) PrintPublicDeclaration(w io.Writer) {
	// Special case to allow SUCCESS Result to be treated as nil error. Must be separately defined as var, not const
	if v.resolvedType.RegistryName() != "VkResult" || v.PublicName() != "SUCCESS" {
		fmt.Fprintf(w, "%s %s = %s", v.PublicName(), v.resolvedType.PublicName(), v.ValueString())
		if v.comment != "" {
			fmt.Fprintf(w, " // %s\n", v.comment)
		} else {
			fmt.Fprintln(w)
		}
	} else if v.IsAlias() {
		fmt.Fprintf(w, "%s = %s\n", v.PublicName(), v.ValueString())
	}
	// else {
	// 	fmt.Fprintf(w, "%s %s = %s\n", v.PublicName(), v.resolvedType.PublicName(), v.ValueString())
	// }

}

func ReadApiConstantsFromXML(doc *xmlquery.Node, externalType TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	var selector string
	if externalType == nil {
		selector = "//enums[@name='API Constants']/enum[not(@type)]"
	} else {
		selector = fmt.Sprintf("//enums[@name='API Constants']/enum[@type='%s']", externalType.RegistryName())
	}
	for _, node := range xmlquery.Find(doc, selector) {
		td := externalType
		if td == nil && node.SelectAttr("alias") == "" {
			// Registry files before API constants were typed (before 1.2.131 or so) give only a value; infer the type
			// from it. Aliases are left alone and take their type from the target.
			td = tr[inferConstantType(node.SelectAttr("value"))]
		}
		valDef := NewEnumValueFromXML(td, node)
		valDef.isCore = true
		vr[valDef.RegistryName()] = valDef
	}
}

// aliasTarget returns the registry name of the value that the enum node is an alias of, or "". Registry files before the
// alias attribute (1.0.69 and earlier) give an alias as value="VK_OTHER_ENUM".
func aliasTarget(elt *xmlquery.Node) string {
	if alias := elt.SelectAttr("alias"); alias != "" {
		return alias
	}
	if value := elt.SelectAttr("value"); strings.HasPrefix(value, "VK_") {
		return value
	}
	return ""
}

var rxFloatConstant = regexp.MustCompile(`^\d+\.\d+[fF]$`)

var rxComplementConstant = regexp.MustCompile(`^\(~(\d+)(U|ULL)(?:-(\d+))?\)$`)

// cConstantToGo converts the C unsigned complement forms that registry files use for constants, "(~0U)", "(~1U)" and
// "(~0U-1)", and their 64-bit "ULL" equivalents, to Go expressions. Any other value is returned as it is. The names in
// exceptions.json are applied after this, and take precedence.
func cConstantToGo(value string) string {
	if rxFloatConstant.MatchString(value) {
		return value[:len(value)-1] // C's f suffix is not valid in Go
	}
	m := rxComplementConstant.FindStringSubmatch(value)
	if m == nil {
		return value
	}
	goType := "uint32"
	if m[2] == "ULL" {
		goType = "uint64"
	}
	n, _ := strconv.Atoi(m[1])
	if m[3] != "" {
		sub, _ := strconv.Atoi(m[3])
		n += sub
	}
	return fmt.Sprintf("^%s(%d)", goType, n)
}

// inferConstantType returns the registry name of the C type for an untyped API constant, from the form of its value,
// following the types that later registry versions give the same constants: a floating point literal is a float, "~0ULL"
// is a uint64_t, and everything else is a uint32_t.
func inferConstantType(value string) string {
	switch {
	case strings.HasSuffix(value, "f") || strings.HasSuffix(value, "F"):
		return "float"
	case strings.Contains(value, "ULL"):
		return "uint64_t"
	default:
		return "uint32_t"
	}
}

func ReadEnumValuesFromXML(doc *xmlquery.Node, td TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	groupSearchNodes := xmlquery.Find(doc, fmt.Sprintf("//enums[@name='%s']", td.RegistryName()))

	for _, groupNode := range groupSearchNodes {
		coreVals := xmlquery.Find(groupNode, "/enum")
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
			}
		}
	}
}

func NewEnumValueFromXML(td TypeDefiner, elt *xmlquery.Node) *enumValue {
	rval := enumValue{}

	alias := aliasTarget(elt)
	if alias == "" {
		rval.registryName = elt.SelectAttr("name")
		rval.valueString = cConstantToGo(elt.SelectAttr("value"))
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

		// Only applies when an extension is promoted to core, otherwise the Extensioner needs to add the extension number to the value
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
	// Very similar to enumValue.Resolve(), but we exclude the type resolution (extenValues are untyped, otherwise
	// they'd be enumValues)
	if v.isResolved {
		return NewIncludeSet()
	}

	rval := NewIncludeSet()

	v.isResolved = true

	if v.IsAlias() {
		v.resolvedAliasValue = vr[v.aliasValueName]
		rval.MergeWith(v.resolvedAliasValue.Resolve(tr, vr))
		v.valueString = RenameIdentifier(v.ValueString())
	}

	rval.IncludeValues[v.registryName] = true
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
	if v.IsAlias() {
		return v.resolvedAliasValue.PublicName()
	} else {
		return v.valueString
	}
}

func NewUntypedEnumValueFromXML(elt *xmlquery.Node) *extenValue {
	rval := extenValue{}

	alias := aliasTarget(elt)
	if alias == "" {
		rval.registryName = elt.SelectAttr("name")
		rval.valueString = elt.SelectAttr("value")
	} else {
		rval.registryName = elt.SelectAttr("name")
		rval.aliasValueName = alias
	}
	rval.comment = elt.SelectAttr("comment")

	return &rval
}

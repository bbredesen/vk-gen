package def

import (
	"fmt"
	"io"

	"github.com/antchfx/xmlquery"
)

type bitmaskValue struct {
	genericValue

	bitposString string

	comment string

	belongsToTypeName string
}

func (v *bitmaskValue) ValueString() string {
	if v.IsAlias() {
		return v.resolvedAliasValue.PublicName()
	} else if v.bitposString != "" {
		return fmt.Sprintf("1 << %s", v.bitposString)
	} else {
		return v.valueString
	}
}

func (v *bitmaskValue) PrintPublicDeclaration(w io.Writer, withExplicitType bool) {
	if withExplicitType {
		fmt.Fprintf(w, "%s %s = %s\n", v.PublicName(), v.resolvedType.PublicName(), v.ValueString())
	} else {
		fmt.Fprintf(w, "%s = %s\n", v.PublicName(), v.ValueString())
	}
}

func (v *bitmaskValue) Resolve(tr TypeRegistry, vr ValueRegistry) {
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

func NewBitmaskValueFromXML(forBitmask TypeDefiner, elt *xmlquery.Node) *bitmaskValue {
	rval := bitmaskValue{}

	alias := elt.SelectAttr("alias")
	if alias == "" {
		rval.registryName = elt.SelectAttr("name")
		rval.valueString = elt.SelectAttr("value")
		rval.bitposString = elt.SelectAttr("bitpos")
	} else {
		rval.registryName = elt.SelectAttr("name")
		rval.aliasValueName = alias
	}
	rval.underlyingTypeName = forBitmask.RegistryName()

	return &rval
}

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

func (v *bitmaskValue) PrintPublicDeclaration(w io.Writer) {
	fmt.Fprintf(w, "%s %s = %s\n", v.PublicName(), v.resolvedType.PublicName(), v.ValueString())
}

func (v *bitmaskValue) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if v.isResolved {
		return NewIncludeSet()
	}

	var rval *IncludeSet

	if v.IsAlias() {
		v.resolvedAliasValue = vr[v.aliasValueName]
		rval = v.resolvedAliasValue.Resolve(tr, vr)
		v.valueString = RenameIdentifier(v.ValueString())

		v.resolvedType = v.resolvedAliasValue.ResolvedType()
		rval.MergeWith(v.resolvedType.Resolve(tr, vr))
	} else {
		v.resolvedType = tr[v.underlyingTypeName]
		rval = v.resolvedType.Resolve(tr, vr)
	}

	rval.IncludeValues[v.registryName] = true
	rval.ResolvedValues[v.registryName] = v

	v.isResolved = true
	return rval
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

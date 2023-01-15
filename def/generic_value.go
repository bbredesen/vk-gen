package def

import (
	"fmt"
	"io"
)

type genericValue struct {
	registryName string
	valueString  string

	underlyingTypeName string
	resolvedType       TypeDefiner

	aliasValueName     string
	resolvedAliasValue ValueDefiner

	extNumber int
	offset    int
	direction int

	isResolved bool
	isCore     bool
}

func (v *genericValue) RegistryName() string { return v.registryName }
func (v *genericValue) PublicName() string   { return RenameIdentifier(v.registryName) }
func (v *genericValue) ValueString() string {
	if v.IsAlias() {
		return v.resolvedAliasValue.PublicName()
	} else {
		return v.valueString
	}
}

func (v *genericValue) SetExtensionNumber(extNum int) { v.extNumber = extNum }

func (v *genericValue) ResolvedType() TypeDefiner { return v.resolvedType }

func (v *genericValue) IsAlias() bool { return v.aliasValueName != "" }
func (v *genericValue) IsCore() bool  { return v.isCore }

func (v *genericValue) Resolve(tr TypeRegistry, vr ValueRegistry) {
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

func (v *genericValue) PrintPublicDeclaration(w io.Writer, withExplicitType bool) {
	if withExplicitType {
		fmt.Fprintf(w, "%s %s = %s\n", v.PublicName(), v.resolvedType.PublicName(), v.ValueString())
	} else {
		fmt.Fprintf(w, "%s = %s\n", v.PublicName(), v.ValueString())
	}
}

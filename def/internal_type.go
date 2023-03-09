package def

import (
	"fmt"
	"io"
)

type internalType struct {
	genericType

	underlyingTypeName, requiresTypeName string
	underlyingType, requiresType         TypeDefiner
}

func (t *internalType) IsIdenticalPublicAndInternal() bool { return true }

func (t *internalType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	rval := t.genericType.Resolve(tr, vr)

	var ok bool
	if t.underlyingType, ok = tr[t.underlyingTypeName]; ok {
		rval.MergeWith(t.underlyingType.Resolve(tr, vr))
	}

	if t.requiresType, ok = tr[t.requiresTypeName]; ok {
		rval.MergeWith(t.requiresType.Resolve(tr, vr))
	}

	rval.ResolvedTypes[t.registryName] = t

	t.isResolved = true
	return rval
}

func (t *internalType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	if t.IsAlias() {
		fmt.Fprintf(w, "type %s = %s\n", t.PublicName(), t.resolvedAliasType.PublicName())
	} else {
		fmt.Fprintf(w, "type %s %s\n", t.PublicName(), t.underlyingType.PublicName())
	}
}

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

func (t *internalType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return &includeSet{}
	}

	rval := t.genericType.Resolve(tr, vr)

	var ok bool
	if t.underlyingType, ok = tr[t.underlyingTypeName]; ok {
		rval.MergeWith(t.genericType.Resolve(tr, vr))
	}

	if t.requiresType, ok = tr[t.requiresTypeName]; ok {
		rval.MergeWith(t.genericType.Resolve(tr, vr))
	}

	return rval
}

func (t *internalType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	if t.comment != "" {
		fmt.Fprintln(w, "// ", t.comment)
	}

	if t.IsAlias() {
		fmt.Fprintf(w, "type %s = %s\n", t.PublicName(), t.resolvedAliasType.PublicName())
	} else {
		fmt.Fprintf(w, "type %s %s\n", t.PublicName(), t.underlyingType.PublicName())
	}

	// TODO print values
	// sort.Sort(byValue(t.values))

	// if len(t.values) > 0 {
	// 	fmt.Fprint(w, "const (\n")
	// 	for _, v := range t.values {
	// 		v.PrintPublicDeclaration(w, !v.IsAlias())
	// 	}
	// 	fmt.Fprint(w, ")\n\n")
	// }
}

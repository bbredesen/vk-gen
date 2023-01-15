package def

// simpleType is a base definition for any Vulkan type that maps an external
// type.
type simpleType struct {
	genericType

	// underlying type is the registered C type, NOT the Go type
	underlyingTypeName     string
	resolvedUnderlyingType TypeDefiner
}

// Category returns CatNone as a simpleType should not be directly allocated.
// Instead, it should be embedded in other simple types, like basetype or enum
func (t *simpleType) Category() TypeCategory { return CatNone }

func (t *simpleType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return &includeSet{}
	}

	is := t.genericType.Resolve(tr, vr)
	t.resolvedUnderlyingType = tr[t.underlyingTypeName]
	is.MergeWith(t.resolvedUnderlyingType.Resolve(tr, vr))

	t.isResolved = true
	return is
}

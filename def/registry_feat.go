package def

import (
	"io"
)

func NewIncludeSet() *IncludeSet {
	return &IncludeSet{
		IncludeTypes:   make(map[string]bool),
		IncludeValues:  make(map[string]bool),
		ResolvedTypes:  make(TypeRegistry),
		ResolvedValues: make(ValueRegistry),
	}
}

type IncludeSet struct {
	IncludeTypes, IncludeValues map[string]bool
	ResolvedTypes               TypeRegistry
	ResolvedValues              ValueRegistry
	// resolvedTypes                       TypeRegistry
}

func (is *IncludeSet) MergeWith(js *IncludeSet) {
	for k := range js.IncludeTypes {
		is.IncludeTypes[k] = true
	}
	for k := range js.IncludeValues {
		is.IncludeValues[k] = true
	}

	for k, v := range js.ResolvedTypes {
		is.ResolvedTypes[k] = v
	}
	for k, v := range js.ResolvedValues {
		is.ResolvedValues[k] = v
	}
}

type includeSet struct {
	includeTypeNames, includeValueNames map[string]bool
	resolvedTypes                       TypeRegistry
}

func (is *includeSet) getIncludeSet() *includeSet {
	return is
}

func (is *includeSet) pushTypes(typeMap map[string]bool) {
	for n := range typeMap {
		is.includeTypeNames[n] = true
	}
}

func (is *includeSet) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	is.resolvedTypes = make(TypeRegistry)

	for tn := range is.includeTypeNames {
		t := tr[tn]
		// newSet := t.Resolve(tr, vr)
		is.resolvedTypes[t.RegistryName()] = t

		// is.MergeWith(newSet)
		// reqsTypes, reqsVals := rb.requiresTypeNames, rb.requiresEnumNames
		// _ = reqsVals
		// is.pushTypes(reqsTypes)
	}

	for _, vd := range vr {
		if vd.IsCore() {
			vd.Resolve(tr, vr)
		}
	}

	for vn := range is.includeValueNames {
		v := vr[vn]
		// vd := vr[vn]
		v.Resolve(tr, vr)
		// is.resolvedTypes[vd.ResolvedType().RegistryName()] = vd.ResolvedType()
	}

	return is
}

func (is *includeSet) ResolvedTypes() TypeRegistry { return is.resolvedTypes }

func (is *includeSet) Platform() string { return "" }

func (is *includeSet) FilenameFragment() string           { return "" }
func (is *includeSet) WriteBuildTags(io.Writer)           {}
func (is *includeSet) IsIdenticalPublicAndInternal() bool { return true }

package def

import (
	"io"
	"strings"

	"github.com/antchfx/xmlquery"
)

type FeatureMap map[string]FeatureCollection

type FeatureCollection interface {
	Resolver
	ResolvedTypes() TypeRegistry
	// AllValues() ValueRegistry

	FilterTypesByCategory() map[TypeCategory]FeatureCollection

	Platform() string
	FilenameFragment() string
	WriteBuildTags(io.Writer)

	MergeWith(FeatureCollection)

	getIncludeSet() *includeSet
}

// func TestingIncludes(tr TypeRegistry) *IncludeSet {
// 	rval := includeSet{}
// 	for k := range tr {
// 		rval.includeTypeNames = append(rval.includeTypeNames, k)
// 	}
// 	return &rval
// }

// func NewIncludeSet() *IncludeSet {
// 	is := includeSet{}
// 	is.includeTypeNames = make(map[string]bool)
// 	is.includeValueNames = make(map[string]bool)
// 	return &is
// }

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

// includeSet is the basic implementation of a FeatureCollection. Several other
// variants of FeatureCollection embed includeSet and are defined below.
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

func (is *includeSet) MergeWith(other FeatureCollection) {
	if other != nil {
		otherIs := other.getIncludeSet()
		is.pushTypes(otherIs.includeTypeNames)
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

func (is *includeSet) FilterTypesByCategory() map[TypeCategory]FeatureCollection {
	rval := make(map[TypeCategory]FeatureCollection)

	for _, t := range is.resolvedTypes {
		cc := rval[t.Category()]
		if cc == nil {
			// cc = &categorySet{
			// 	cat:        t.Category(),
			// 	IncludeSet: NewIncludeSet(),
			// }
			// rval[t.Category()] = cc
		}

		cc.ResolvedTypes()[t.RegistryName()] = t
	}

	return rval
}

func (is *includeSet) Platform() string { return "" }

func (is *includeSet) FilenameFragment() string           { return "" }
func (is *includeSet) WriteBuildTags(io.Writer)           {}
func (is *includeSet) IsIdenticalPublicAndInternal() bool { return true }

type featureSet struct {
	apiName, featureName, featureNumber string
	includeSet
}

func (fs *featureSet) getIncludeSet() *includeSet {
	return &fs.includeSet
}

func ReadFeatureFromXML(featureNode *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) *featureSet {
	rval := featureSet{}
	// rval.includeSet = *NewIncludeSet()

	// Manual include, type not in vk.xml
	rval.includeTypeNames["uintptr_t"] = true

	for _, reqNode := range xmlquery.Find(featureNode, "/require") {
		for _, typeNode := range xmlquery.Find(reqNode, "/type") { //} or /command") {
			rval.includeTypeNames[typeNode.SelectAttr("name")] = true
		}

		for _, cmdNode := range xmlquery.Find(reqNode, "/command") {
			rval.includeTypeNames[cmdNode.SelectAttr("name")] = true
		}

		for _, enumNode := range xmlquery.Find(reqNode, "/enum") {
			regTypeName := enumNode.SelectAttr("extends")
			if regTypeName == "" {
				// Not handling the embedded extension number and name (for now, at least)
				continue
			}

			td := tr[regTypeName]

			vd := NewEnumValueFromXML(td, enumNode)

			// TODO MERGE VALUES
			vr[vd.RegistryName()] = vd
			rval.includeValueNames[vd.RegistryName()] = true
		}
	}

	return &rval
}

type categorySet struct {
	cat TypeCategory
	*IncludeSet
}

func (cc *categorySet) FilenameFragment() string {
	return strings.ToLower(strings.TrimPrefix(cc.cat.String(), "Cat"))
}

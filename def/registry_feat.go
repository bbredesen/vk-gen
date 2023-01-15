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

func TestingIncludes(tr TypeRegistry) *includeSet {
	rval := includeSet{}
	for k := range tr {
		rval.includeTypeNames = append(rval.includeTypeNames, k)
	}
	return &rval
}

// includeSet is the basic implementation of a FeatureCollection. Several other
// variants of FeatureCollection embed includeSet and are defined below.
type includeSet struct {
	includeTypeNames, includeValueNames []string
	resolvedTypes                       TypeRegistry
}

func (is *includeSet) getIncludeSet() *includeSet {
	return is
}

func (is *includeSet) pushTypes(typeName []string) {
	is.includeTypeNames = append(is.includeTypeNames, typeName...)
}

func (is *includeSet) MergeWith(other FeatureCollection) {
	if other != nil {
		otherIs := other.getIncludeSet()
		is.pushTypes(otherIs.includeTypeNames)
	}
}

func (is *includeSet) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	is.resolvedTypes = make(TypeRegistry)

	for i := 0; i < len(is.includeTypeNames); i++ {
		t := tr[is.includeTypeNames[i]]
		newSet := t.Resolve(tr, vr)
		is.resolvedTypes[t.RegistryName()] = t

		is.MergeWith(newSet)
		// reqsTypes, reqsVals := rb.requiresTypeNames, rb.requiresEnumNames
		// _ = reqsVals
		// is.pushTypes(reqsTypes)
	}

	for _, vd := range vr {
		if vd.IsCore() {
			vd.Resolve(tr, vr)
		}
	}

	for _, vn := range is.includeValueNames {
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
			cc = &categorySet{
				cat: t.Category(),
				includeSet: includeSet{
					resolvedTypes: make(TypeRegistry),
				},
			}
			rval[t.Category()] = cc
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

func ReadFeatureFromXML(featureNode *xmlquery.Node) *featureSet {
	rval := featureSet{}

	rval.includeTypeNames = append(rval.includeTypeNames, "uintptr_t")

	for _, reqNode := range xmlquery.Find(featureNode, "/require") {
		for _, typeNode := range xmlquery.Find(reqNode, "/type") { //} or /command") {
			rval.includeTypeNames = append(rval.includeTypeNames, typeNode.SelectAttr("name"))
		}

		for _, cmdNode := range xmlquery.Find(reqNode, "/command") {
			rval.includeTypeNames = append(rval.includeTypeNames, cmdNode.SelectAttr("name"))
		}

		for _, enumNode := range xmlquery.Find(reqNode, "/enum") {
			rval.includeValueNames = append(rval.includeValueNames, enumNode.SelectAttr("name"))
		}
	}

	return &rval
}

type categorySet struct {
	cat TypeCategory
	includeSet
}

func (cc *categorySet) FilenameFragment() string {
	return strings.ToLower(strings.TrimPrefix(cc.cat.String(), "Cat"))
}

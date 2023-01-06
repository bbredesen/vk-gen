package def

import (
	"fmt"
	"io"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type includeSet struct {
	includeTypeNames, includeValueNames []string
	resolvedTypes                       TypeRegistry
}

func (is *includeSet) pushTypes(typeName []string) {
	is.includeTypeNames = append(is.includeTypeNames, typeName...)
}

func (is *includeSet) mergeWith(other *includeSet) {
	if other != nil {
		is.pushTypes(other.includeTypeNames)
	}

}

func (is *includeSet) Resolve(tr TypeRegistry, vr ValueRegistry) {
	is.resolvedTypes = make(TypeRegistry)

	for i := 0; i < len(is.includeTypeNames); i++ {
		t := tr[is.includeTypeNames[i]]
		newSet := t.Resolve(tr, vr)
		is.resolvedTypes[t.RegistryName()] = t

		is.mergeWith(newSet)
		// reqsTypes, reqsVals := rb.requiresTypeNames, rb.requiresEnumNames
		// _ = reqsVals
		// is.pushTypes(reqsTypes)
	}

	for _, vd := range vr {
		// vd := vr[vn]
		vd.Resolve(tr, vr)
		// is.resolvedTypes[vd.ResolvedType().RegistryName()] = vd.ResolvedType()
	}
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

func (is *includeSet) Filename() string         { return "" }
func (is *includeSet) WriteBuildTags(io.Writer) {}

type FeatureMap map[string]FeatureCollection

type FeatureCollection interface {
	Resolve(TypeRegistry, ValueRegistry)

	ResolvedTypes() TypeRegistry

	FilterTypesByCategory() map[TypeCategory]FeatureCollection

	Platform() string
	Filename() string
	WriteBuildTags(io.Writer)
}

type featureSet struct {
	apiName, featureName, featureNumber string
	includeSet
}

type platformSet struct {
	platformName string
	goBuildTag   string
	goImports    []string

	includeSet
}

type extensionSet struct {
	extName, extType string
	extNumber        int

	includeSet
}

type categorySet struct {
	cat TypeCategory
	includeSet
}

func (cc *categorySet) Filename() string {
	fnbase := strings.ToLower(strings.TrimPrefix(cc.cat.String(), "Cat"))
	// if cc.Platform() == "" {
	return fmt.Sprintf("%s.go", fnbase)
	// } else {
	// return fmt.Sprintf("%s_%s.go", fnbase, cc.Platform())
	// }
}

// func (b *requireBlock) ResolvedTypes() TypeRegistry { return b.resolvedTypes }
// func (b *requireBlock) Filename() string            { return "!invalid" }
// func (b *requireBlock) WriteBuildTags(io.Writer)    { /* NOP */ }
// func (b *requireBlock) Platform() string            { return "" }

// func (b *requireBlock) Resolve(tr TypeRegistry, vr ValueRegistry) {
// 	b.resolvedTypes = make(TypeRegistry)

// 	addlRequired := &requireBlock{}

// 	for _, n := range b.requiresTypeNames {
// 		tn, found := tr[n]
// 		if !found {
// 			logrus.WithField("required type", n).Errorf("required type not found in regsitry")
// 			continue
// 		}
// 		b.resolvedTypes[n] = tn
// 		addlRequired.mergeWith(tn.Resolve(tr, vr))
// 	}

// 	for _, n := range b.requiresEnumNames {
// 		en, found := vr[n]
// 		if !found {
// 			logrus.WithField("required enum name", n).Errorf("required value not found in regsitry")
// 			continue
// 		}
// 		// b.resolvedTypes[n] = en
// 		en.Resolve(tr, vr)
// 	}

// 	for _, val := range vr {
// 		val.Resolve(tr, vr)
// 	}

// 	if len(addlRequired.requiresTypeNames) == 0 {
// 		return
// 	} else {
// 		addlRequired.Resolve(tr, vr)
// 	}
// }

func (p *platformSet) RegistryName() string { return p.platformName }
func (p *platformSet) Platform() string     { return p.platformName }
func (p *platformSet) WriteBuildTags(w io.Writer) {
	if p.goBuildTag != "" {
		fmt.Fprintf(w, "//go:build %s\n", p.goBuildTag)
	}
}

func (s *platformSet) Resolve(tr TypeRegistry, vr ValueRegistry) {
	// for _, t := range tr {
	// 	// In practice, this will only pick up include types and extensions
	// 	if t.Platform() == s.platformName {
	// 		s.resolvedTypes[t.RegistryName()] = t
	// 		t.Resolve(tr, vr)
	// 	}
	// }

	// for _, v := range vr {
	// 	if v.Platform() == s.platformName {
	// 		v.Resolve(tr, vr)
	// 	}
	// }
}

func ReadPlatformsFromXML(doc *xmlquery.Node) FeatureMap {
	rval := make(FeatureMap)
	// rval := make([]FeatureCollection, 0)
	for _, pnode := range xmlquery.Find(doc, "//platforms/platform") {
		s := newPlatformSetFromXML(pnode)
		rval[s.platformName] = s
	}
	return rval
}

func newPlatformSetFromXML(node *xmlquery.Node) *platformSet {
	rval := platformSet{}

	rval.platformName = node.SelectAttr("name")
	rval.resolvedTypes = make(TypeRegistry)
	// rval.resolvedValues = make(ValueRegistry)
	// rval.comment = node.SelectAttr("comment")

	return &rval
}

func ReadPlatformExceptionsFromJSON(exceptions gjson.Result, fm FeatureMap) {
	exceptions.Get("platform").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "comment" {
			return true
		} // Ignore comments

		var ps *platformSet
		if entry := fm[key.String()]; entry != nil {
			ps = entry.(*platformSet)
		}
		fm[key.String()] = newOrUpdatePlatformTypeFromJSON(key.String(), exVal, ps)
		return true
	})
}

func newOrUpdatePlatformTypeFromJSON(key string, exception gjson.Result, existing *platformSet) *platformSet {
	var updatedEntry *platformSet
	if existing == nil {
		logrus.WithField("registry type", key).Info("no existing registry entry for platform type")
		updatedEntry = &platformSet{}
		updatedEntry.platformName = key
	} else {
		updatedEntry = existing
	}

	// static mapping vk platform to go build tags
	updatedEntry.goBuildTag = exception.Get("go:build").String()
	exception.Get("go:imports").ForEach(func(_, val gjson.Result) bool {
		updatedEntry.goImports = append(updatedEntry.goImports, val.String())
		return true
	})

	return updatedEntry
}

func ReadFeatureSetFromXML(node *xmlquery.Node) FeatureCollection {
	rval := featureSet{}
	rval.apiName = node.SelectAttr("api")
	rval.featureName = node.SelectAttr("name")
	rval.featureNumber = node.SelectAttr("number")

	for _, typeNode := range xmlquery.Find(node, "require/type") {
		rval.includeTypeNames = append(rval.includeTypeNames, typeNode.SelectAttr("name"))
	}

	for _, typeNode := range xmlquery.Find(node, "require/command") {
		rval.includeTypeNames = append(rval.includeTypeNames, typeNode.SelectAttr("name"))
	}

	for _, valNode := range xmlquery.Find(node, "require/enum") {
		rval.includeValueNames = append(rval.includeValueNames, valNode.SelectAttr("name"))
	}

	return &rval
}

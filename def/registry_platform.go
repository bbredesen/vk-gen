package def

import (
	"fmt"
	"io"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type platformSet struct {
	includeSet

	platformName string
	goBuildTag   string
	goImports    []string
}

func (p *platformSet) RegistryName() string { return p.platformName }
func (p *platformSet) Platform() string     { return p.platformName }
func (p *platformSet) WriteBuildTags(w io.Writer) {
	if p.goBuildTag != "" {
		fmt.Fprintf(w, "//go:build %s\n", p.goBuildTag)
	}
}
func (p *platformSet) FilenameFragment() string {
	if p.goBuildTag != "" {
		return p.platformName
	} else {
		return ""
	}
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

	return &rval
}

func ReadPlatformExceptionsFromJSON(exceptions gjson.Result, fm FeatureMap) {
	exceptions.Get("platform").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
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
		logrus.WithField("registry type", key).Warn("no existing registry entry for platform type in exceptions.json")
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

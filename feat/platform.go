package feat

import (
	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type PlatformRegistry map[string]*Platform

type Platform struct {
	platformName string
	comment      string

	GoBuildTag string
	GoImports  []string

	platformExtensionNames map[string]bool
	extensions             map[string]*Extension
}

func NewGeneralPlatform() *Platform {
	rval := Platform{
		platformName:           "",
		comment:                "Empty platform for general Vulkan types and commands",
		platformExtensionNames: map[string]bool{},
		extensions:             map[string]*Extension{}, // Probably pivot extension type to be a feature
	}
	return &rval
}

func NewPlatformFromXML(plNode *xmlquery.Node) *Platform {
	rval := Platform{
		platformName:           plNode.SelectAttr("name"),
		comment:                plNode.SelectAttr("comment"),
		platformExtensionNames: map[string]bool{},
		extensions:             map[string]*Extension{},
	}
	return &rval
}

func NewOrUpdatePlatformFromJSON(key string, exception gjson.Result, existing *Platform) *Platform {
	var updatedEntry *Platform = existing
	if existing == nil {
		logrus.WithField("registry type", key).Warn("no existing registry entry for platform type in exceptions.json")
		updatedEntry = &Platform{
			platformName:           key,
			comment:                exception.Get("comment").String(),
			platformExtensionNames: map[string]bool{},
			extensions:             map[string]*Extension{},
		}
	}

	// static mapping vk platform to go build tags
	updatedEntry.GoBuildTag = exception.Get("go:build").String()
	exception.Get("go:imports").ForEach(func(_, val gjson.Result) bool {
		updatedEntry.GoImports = append(updatedEntry.GoImports, val.String())
		return true
	})

	return updatedEntry
}

func (p *Platform) Name() string { return p.platformName }

func (p *Platform) IncludeExtension(e *Extension) {
	p.extensions[e.Name()] = e
}

func (p *Platform) Extensions() map[string]*Extension {
	return p.extensions
}

func (p *Platform) GeneratePlatformFeatures() *Feature {
	rval := NewFeature()
	for _, ext := range p.extensions {
		rval.MergeWith(ext.Feature)
	}

	return rval
}

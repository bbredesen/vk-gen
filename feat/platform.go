package feat

import (
	"fmt"

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
	if updatedEntry.GoBuildTag == "" {
		logrus.WithField("registry type", key).Warn("no go:build tag for platform type in exceptions.json")
	}

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

// legacyProtectPlatforms maps the preprocessor macro that guards a platform's extensions to the platform name. Registry
// files before the <platforms> section (1.0.69 and earlier) have no platform attribute on an extension, only protect.
var legacyProtectPlatforms = map[string]string{
	"VK_USE_PLATFORM_ANDROID_KHR":     "android",
	"VK_USE_PLATFORM_IOS_MVK":         "ios",
	"VK_USE_PLATFORM_MACOS_MVK":       "macos",
	"VK_USE_PLATFORM_METAL_EXT":       "metal",
	"VK_USE_PLATFORM_MIR_KHR":         "mir",
	"VK_USE_PLATFORM_VI_NN":           "vi",
	"VK_USE_PLATFORM_WAYLAND_KHR":     "wayland",
	"VK_USE_PLATFORM_WIN32_KHR":       "win32",
	"VK_USE_PLATFORM_XCB_KHR":         "xcb",
	"VK_USE_PLATFORM_XLIB_KHR":        "xlib",
	"VK_USE_PLATFORM_XLIB_XRANDR_EXT": "xlib_xrandr",
}

// PlatformExtensionsXPath selects the extensions for a platform: those with the platform attribute and, for registry
// files that predate it, those guarded by the platform's protect macro.
func PlatformExtensionsXPath(platformName string) string {
	xpath := fmt.Sprintf("//extension[@platform='%s'", platformName)
	for protect, name := range legacyProtectPlatforms {
		if name == platformName {
			xpath += fmt.Sprintf(" or (not(@platform) and @protect='%s')", protect)
		}
	}
	return xpath + "]"
}

// CoreExtensionsXPath selects the extensions that are not specific to a platform and are supported by the named API (the
// supported attribute is a comma-separated list, for example supported="vulkan,vulkansc").
func CoreExtensionsXPath(apiName string) string {
	return fmt.Sprintf("//extension[not(@platform) and not(starts-with(@protect, 'VK_USE_PLATFORM_')) and "+
		"contains(concat(',', @supported, ','), ',%s,')]", apiName)
}

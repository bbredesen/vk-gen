package feat

import (
	"strconv"

	"github.com/antchfx/xmlquery"
	"github.com/bbredesen/vk-gen/def"
	"github.com/sirupsen/logrus"
)

type Extension struct {
	extensionName                   string
	extensionNumber                 string
	supportedString, platformString string

	*Feature

	requireExtensionNames map[string]bool
}

func ReadExtensionFromXML(extNode *xmlquery.Node, tr def.TypeRegistry, vr def.ValueRegistry) *Extension {
	rval := Extension{
		extensionName:         extNode.SelectAttr("name"),
		extensionNumber:       extNode.SelectAttr("number"),
		supportedString:       extNode.SelectAttr("supported"),
		platformString:        extensionPlatform(extNode),
		requireExtensionNames: make(map[string]bool),
		Feature:               NewFeature(),
	}

	for _, reqNode := range xmlquery.Find(extNode, "/require") {
		for _, typeNode := range xmlquery.Find(reqNode, "/type") { //} or /command") {
			rval.requireTypeNames[typeNode.SelectAttr("name")] = true
		}

		for _, cmdNode := range xmlquery.Find(reqNode, "/command") {
			rval.requireTypeNames[cmdNode.SelectAttr("name")] = true
		}

		for _, enumNode := range xmlquery.Find(reqNode, "/enum") {
			extendsTypeName := enumNode.SelectAttr("extends")

			if extendsTypeName == "" && enumNode.SelectAttr("value") == "" && enumNode.SelectAttr("alias") == "" {
				// Some extensions are actually requiring an outside constant, like VK_SHADER_UNUSED_KHR; These
				// should already be in the registry as external types
				rval.requireValueNames[enumNode.SelectAttr("name")] = true
				continue
			}

			extNum, err := strconv.Atoi(rval.extensionNumber)
			if err != nil {
				logrus.WithField("extension number", rval.extensionNumber).WithField("error", err.Error()).Fatal("Could not convert extension number to int")
			}

			var vd def.ValueDefiner

			if td, found := tr[extendsTypeName]; found {
				if enumNode.SelectAttr("bitpos") != "" {
					vd = def.NewBitmaskValueFromXML(td, enumNode)
				} else {
					vd = def.NewEnumValueFromXML(td, enumNode)
				}
			} else {
				vd = def.NewUntypedEnumValueFromXML(enumNode)
			}
			vd.SetExtensionNumber(extNum)
			vr[vd.RegistryName()] = vd

			rval.requireValueNames[enumNode.SelectAttr("name")] = true
		}
	}

	return &rval
}

// extensionPlatform returns the extension's platform name, from the platform attribute or, in registry files that
// predate it, from the protect macro. It is "" for an extension that is not platform specific.
func extensionPlatform(extNode *xmlquery.Node) string {
	if platform := extNode.SelectAttr("platform"); platform != "" {
		return platform
	}
	return legacyProtectPlatforms[extNode.SelectAttr("protect")]
}

func (e *Extension) Name() string         { return e.extensionName }
func (e *Extension) PlatformName() string { return e.platformString }

package feat

import (
	"strconv"

	"github.com/antchfx/xmlquery"
	"github.com/bbredesen/vk-gen/def"
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
		platformString:        extNode.SelectAttr("platform"),
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
				panic(err)
			}

			var vd def.ValueDefiner

			if td, found := tr[extendsTypeName]; found {
				vd = def.NewEnumValueFromXML(td, enumNode)
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

func (e *Extension) Name() string         { return e.extensionName }
func (e *Extension) PlatformName() string { return e.platformString }

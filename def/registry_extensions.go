package def

import (
	"strconv"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type extensionSet struct {
	extName, extType string
	extNumber        int

	platformName string

	*IncludeSet
}

func SegmentExtensionsByPlatform(allExts []*extensionSet) map[string][]*extensionSet {
	rval := make(map[string][]*extensionSet)
	for _, ext := range allExts {
		rval[ext.platformName] = append(rval[ext.platformName], ext)
	}
	return rval
}

func ReadAllExtensionsFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) []*extensionSet {
	rval := make([]*extensionSet, 0)

	for _, extNode := range xmlquery.Find(doc, "//extensions/extension") {
		if extNode.SelectAttr("supported") == "disabled" {
			continue
		}

		ext := extensionSet{
			extName:      extNode.SelectAttr("name"),
			extType:      extNode.SelectAttr("type"),
			platformName: extNode.SelectAttr("platform"),
		}
		ext.IncludeSet = NewIncludeSet()

		if num, err := strconv.Atoi(extNode.SelectAttr("number")); err != nil {
			logrus.WithError(err).WithField("extension name", ext.extName).Error("Could not convert number attribute on extension")
			continue
		} else {
			ext.extNumber = num
		}

		for _, reqNode := range xmlquery.Find(extNode, "/require") {
			// Commands and types are referenced in the extension but defined elsewhere, and can be resolved
			// independently. Just add the name to the include list
			for _, typeNode := range xmlquery.Find(reqNode, "/type") { //} or /command") {
				ext.IncludeTypes[typeNode.SelectAttr("name")] = true
			}

			for _, cmdNode := range xmlquery.Find(reqNode, "/command") {
				ext.IncludeTypes[cmdNode.SelectAttr("name")] = true
			}

			// Enum values are actually defined in the extension, though they may be re-defined or partially defined
			// elsewhere if the extension was promoted to core. Add the value name, plus create the ValueDefiner and put
			// it in the registry.
			for _, enumNode := range xmlquery.Find(reqNode, "/enum") {
				// Also, bitmasks and values with offsets are defined in the same place, so first figure out which type
				// of value this is and call the appropriate New... function. (TODO)
				regTypeName := enumNode.SelectAttr("extends")
				if regTypeName == "" {
					// Not handling the embedded extension number and name (for now, at least)
					continue
				}

				td := tr[regTypeName]

				vd := NewEnumValueFromXML(td, enumNode)
				vd.SetExtensionNumber(ext.extNumber)

				// TODO MERGE VALUES
				vr[vd.RegistryName()] = vd
				ext.IncludeValues[vd.RegistryName()] = true
			}
		}

		rval = append(rval, &ext)
	}

	return rval
}

func (es *extensionSet) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	// es.resolvedTypes = make(TypeRegistry)

	// for tn := range es.includeTypeNames {
	// 	t := tr[tn]
	// 	newSet := t.Resolve(tr, vr)
	// 	es.resolvedTypes[t.RegistryName()] = t

	// 	es.MergeWith(newSet)
	// }

	// for vn := range es.includeValueNames {
	// 	if v, found := vr[vn]; found {
	// 		v.SetExtensionNumber(es.extNumber)
	// 		v.Resolve(tr, vr)
	// 	}
	// }

	// return &es.includeSet
	return nil
}

func (es *extensionSet) FilenameFragment() string { return "" }

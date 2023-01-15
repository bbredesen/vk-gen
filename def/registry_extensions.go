package def

import (
	"strconv"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
)

type extensionSet struct {
	extName, extType string
	extNumber        int

	includeSet
}

func ReadAllExtensionsFromXML(doc *xmlquery.Node) []*extensionSet {
	rval := make([]*extensionSet, 0)

	for _, extNode := range xmlquery.Find(doc, "//extensions/extension[@name='VK_KHR_surface']") {
		ext := extensionSet{
			extName: extNode.SelectAttr("name"),
			extType: extNode.SelectAttr("type"),
		}

		if num, err := strconv.Atoi(extNode.SelectAttr("number")); err != nil {
			logrus.WithError(err).WithField("extension name", ext.extName).Error("Could not convert number attribute on extension")
			continue
		} else {
			ext.extNumber = num
		}

		for _, reqNode := range xmlquery.Find(extNode, "/require") {
			for _, typeNode := range xmlquery.Find(reqNode, "/type") { //} or /command") {
				ext.includeTypeNames = append(ext.includeTypeNames, typeNode.SelectAttr("name"))
			}

			for _, cmdNode := range xmlquery.Find(reqNode, "/command") {
				ext.includeTypeNames = append(ext.includeTypeNames, cmdNode.SelectAttr("name"))
			}

			for _, enumNode := range xmlquery.Find(reqNode, "/enum") {
				ext.includeValueNames = append(ext.includeValueNames, enumNode.SelectAttr("name"))
			}
		}

		rval = append(rval, &ext)
	}

	return rval
}

func (es *extensionSet) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	es.resolvedTypes = make(TypeRegistry)

	for i := 0; i < len(es.includeTypeNames); i++ {
		t := tr[es.includeTypeNames[i]]
		newSet := t.Resolve(tr, vr)
		es.resolvedTypes[t.RegistryName()] = t

		es.MergeWith(newSet)
	}

	for _, vn := range es.includeValueNames {
		if v, found := vr[vn]; found {
			v.SetExtensionNumber(es.extNumber)
			v.Resolve(tr, vr)
		}
	}

	return &es.includeSet
}

func (es *extensionSet) FilenameFragment() string { return "" }

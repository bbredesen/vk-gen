package def

import (
	"fmt"
	"io"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// includeType is a type required in Vulkan but defined outside of the API. Primarily,
// it is certain primitive types and window-system specific types (HWND or wl_display, for example)
type includeType struct {
	genericType
	goImports []string

	resolvedIncludedTypes TypeRegistry
	includedTypeNames     map[string]bool
}

func (t *includeType) Category() TypeCategory { return CatInclude }

func (t *includeType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	t.resolvedIncludedTypes = make(TypeRegistry)
	rval := &includeSet{}

	for key := range t.includedTypeNames {
		td := tr[key]
		t.resolvedIncludedTypes[key] = td
		td.Resolve(tr, vr)
		rval.includeTypeNames = append(rval.includeTypeNames, key)
	}

	return rval
}

func ReadIncludeTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, _ ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//type[@category='include']") {
		typ := NewIncludeTypeFromXML(node)
		if tr[typ.RegistryName()] != nil {
			logrus.WithField("registry name", typ.RegistryName()).Warn("Overwriting include type in registry")
		}

		for _, incNode := range xmlquery.Find(doc, fmt.Sprintf("//type[@requires='%s']", typ.RegistryName())) {
			newType := NewStaticTypeFromXML(incNode)
			tr[newType.RegistryName()] = newType
			typ.includedTypeNames[newType.RegistryName()] = true
		}

		tr[typ.RegistryName()] = typ
	}
}

func NewIncludeTypeFromXML(node *xmlquery.Node) *includeType {
	rval := includeType{}
	rval.registryName = node.SelectAttr("name")
	rval.publicName = renameIdentifier(rval.registryName)
	rval.resolvedIncludedTypes = make(TypeRegistry)
	rval.includedTypeNames = make(map[string]bool)
	return &rval
}

func ReadIncludeExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("include").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "comment" {
			return true
		} // Ignore comments

		entry := NewOrUpdateIncludeTypeFromJSON(key.String(), exVal, tr, vr)
		tr[key.String()] = entry
		return true
	})
}

func NewOrUpdateIncludeTypeFromJSON(key string, exception gjson.Result, tr TypeRegistry, vr ValueRegistry) TypeDefiner {
	existing := tr[key]
	var updatedEntry *includeType
	if existing == nil {
		logrus.WithField("registry type", key).Info("no existing registry entry for include type")
		updatedEntry = &includeType{}
		updatedEntry.registryName = key
		updatedEntry.publicName = renameIdentifier(key)
	} else {
		updatedEntry = existing.(*includeType)
	}

	exception.Get("go:imports").ForEach(func(_, val gjson.Result) bool {
		updatedEntry.goImports = append(updatedEntry.goImports, val.String())
		return true
	})

	exception.Get("types").ForEach(func(key, typ gjson.Result) bool {
		newTyp := NewStaticTypeFromJSON(key, typ)
		updatedEntry.includedTypeNames[newTyp.RegistryName()] = true
		tr[newTyp.RegistryName()] = newTyp

		typ.Get("constants").ForEach(func(ck, cv gjson.Result) bool {
			newVal := NewConstantValue(ck.String(), cv.String(), newTyp.RegistryName())

			vr[newVal.RegistryName()] = newVal
			return true
		})

		return true
	})

	return updatedEntry
}

func (t *includeType) PrintPublicDeclaration(w io.Writer) {
	// for _, incTyp := range t.resolvedIncludedTypes {
	// 	incTyp.PrintPublicDeclaration(w)
	// }
}

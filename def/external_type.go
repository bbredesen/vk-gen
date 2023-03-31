package def

import (
	"fmt"
	"io"
	"sort"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type externalType struct {
	genericType
	mappedTypeName, requiresTypeName string
	// no mappedType because that type is externally defined in Go, i.e. it is a
	// uint32 or a windows.HWND, for example.
	requiresType TypeDefiner

	translateToPublicOverride, translateToInternalOverride string
}

func (t *externalType) Category() TypeCategory { return CatExternal }
func (t *externalType) IsIdenticalPublicAndInternal() bool {
	return t.translateToPublicOverride == "" && t.translateToInternalOverride == ""
}

func (t *externalType) TranslateToPublic(inputVar string) string {
	if t.translateToPublicOverride != "" {
		return fmt.Sprintf("%s(%s)", t.translateToPublicOverride, inputVar)
	}
	return t.genericType.TranslateToPublic(inputVar)
}

func (t *externalType) TranslateToInternal(inputVar string) string {
	if t.translateToInternalOverride != "" {
		return fmt.Sprintf("%s(%s)", t.translateToInternalOverride, inputVar)
	}
	return t.genericType.TranslateToInternal(inputVar)
}

func ReadExternalTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry, api string) {
	queryString := fmt.Sprintf("//types/type[not(@category) and (@api='%s' or not(@api))]", api)

	for _, node := range xmlquery.Find(doc, queryString) {
		typ := NewExternalTypeFromXML(node)
		if tr[typ.RegistryName()] != nil {
			logrus.WithField("registry name", typ.RegistryName()).Warn("Overwriting external type in registry")
		}

		tr[typ.RegistryName()] = typ
		// Read external enums
		ReadApiConstantsFromXML(doc, typ, tr, vr)
	}
	// Read aliased (untyped) external enums
	ReadApiConstantsFromXML(doc, nil, tr, vr)
}

func NewExternalTypeFromXML(node *xmlquery.Node) *externalType {
	rval := externalType{}
	rval.registryName = node.SelectAttr("name")

	return &rval
}

func ReadExternalExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("external").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
			return true
		} // Ignore comments

		entry := NewOrUpdateExternalTypeFromJSON(key.String(), exVal, tr, vr)
		tr[key.String()] = entry

		return true
	})
}

func NewOrUpdateExternalTypeFromJSON(key string, exception gjson.Result, tr TypeRegistry, vr ValueRegistry) TypeDefiner {
	existing := tr[key]
	var updatedEntry *externalType

	if existing == nil {
		logrus.WithField("registry type", key).Info("no existing registry entry for external type")
		updatedEntry = &externalType{}
		updatedEntry.registryName = key
		updatedEntry.publicName = RenameIdentifier(key)
	} else {
		updatedEntry = existing.(*externalType)
	}

	updatedEntry.mappedTypeName = exception.Get("go:type").String()

	updatedEntry.translateToPublicOverride = exception.Get("go:translatePublic").String()
	updatedEntry.translateToInternalOverride = exception.Get("go:translateInternal").String()

	exception.Get("enums").ForEach(func(key, value gjson.Result) bool {
		NewOrUpdateExternalValueFromJSON(key.String(), value.String(), updatedEntry, tr, vr)
		return true
	})

	return updatedEntry
}

func NewOrUpdateExternalValueFromJSON(key, value string, td TypeDefiner, tr TypeRegistry, vr ValueRegistry) {
	existing := vr[key]
	var updatedEntry *enumValue

	if existing == nil {
		logrus.WithField("registry type", key).Info("no existing registry entry for external type")
		updatedEntry = &enumValue{}
		updatedEntry.registryName = key
	} else {
		updatedEntry = existing.(*enumValue)
	}

	updatedEntry.valueString = value
	updatedEntry.isCore = true

	vr[key] = updatedEntry
}

func (t *externalType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}
	is := t.genericType.Resolve(tr, vr)

	// override naming here so we don't rename keywords like uint32 to asUint32
	t.publicName = trimVk(t.mappedTypeName)
	t.internalName = t.publicName

	if t.requiresTypeName != "" {
		is.MergeWith(t.requiresType.Resolve(tr, vr))
	}

	is.ResolvedTypes[t.registryName] = t

	t.isResolved = true
	return is
}

// PrintPublicDeclaration is for external types just needs to print constants
func (t *externalType) PrintPublicDeclaration(w io.Writer) {

	sort.Sort(ByValue(t.values))

	if len(t.values) > 0 {
		fmt.Fprint(w, "const (\n")
		for _, v := range t.values {
			v.PrintPublicDeclaration(w)
		}
		fmt.Fprint(w, ")\n\n")
	}
}

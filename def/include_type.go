package def

import (
	"fmt"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type includeType struct {
	genericType
	goImports []string

	resolvedIncludedTypes TypeRegistry
	includedTypeNames     map[string]bool
}

func (t *includeType) Category() TypeCategory             { return CatInclude }
func (t *includeType) IsIdenticalPublicAndInternal() bool { return true }

func (t *includeType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	t.resolvedIncludedTypes = make(TypeRegistry)
	rval := NewIncludeSet()

	for key := range t.includedTypeNames {
		td := tr[key]
		t.resolvedIncludedTypes[key] = td
		rval.MergeWith(td.Resolve(tr, vr))
		rval.IncludeTypes[key] = true
	}

	rval.ResolvedTypes[t.registryName] = t

	t.isResolved = true
	return rval
}

func ReadIncludeTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, _ ValueRegistry, api string) {
	queryString := fmt.Sprintf("//types/type[@category='include' and (@api='%s' or @api='')]", api)

	for _, node := range xmlquery.Find(doc, queryString) {
		typ := NewIncludeTypeFromXML(node)
		if tr[typ.RegistryName()] != nil {
			logrus.WithField("registry name", typ.RegistryName()).Warn("Overwriting include type in registry")
		}

		tr[typ.RegistryName()] = typ
	}
}

func NewIncludeTypeFromXML(node *xmlquery.Node) *includeType {
	rval := includeType{}
	rval.registryName = node.SelectAttr("name")
	rval.publicName = RenameIdentifier(rval.registryName)
	rval.resolvedIncludedTypes = make(TypeRegistry)
	rval.includedTypeNames = make(map[string]bool)
	return &rval
}

func ReadIncludeExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("include").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
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
		updatedEntry.publicName = RenameIdentifier(key)
	} else {
		updatedEntry = existing.(*includeType)
	}

	exception.Get("go:imports").ForEach(func(_, val gjson.Result) bool {
		updatedEntry.goImports = append(updatedEntry.goImports, val.String())
		return true
	})

	return updatedEntry
}

func (t *includeType) RegisterImports(reg map[string]bool) {
	reg[t.PublicName()] = true
}

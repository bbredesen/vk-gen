package def

import (
	"fmt"
	"io"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type baseType struct {
	simpleType

	publicTypeNameOverride string

	translateToPublicOverride, translateToInternalOverride string
}

func (t *baseType) Category() TypeCategory { return CatBasetype }

func (t *baseType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	rval := t.simpleType.Resolve(tr, vr)

	rval.ResolvedTypes[t.registryName] = t

	t.isResolved = true
	return rval
}

func (t *baseType) PublicName() string {
	if t.publicTypeNameOverride != "" {
		return t.publicTypeNameOverride
	}
	return t.simpleType.PublicName()
}

func (t *baseType) IsIdenticalPublicAndInternal() bool {
	return t.publicTypeNameOverride == ""
}

func (t *baseType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)
	if t.publicTypeNameOverride != "" {
		return
	} else {
		fmt.Fprintf(w, "type %s %s\n", t.PublicName(), t.resolvedUnderlyingType.InternalName())
	}
}

func (t *baseType) PrintInternalDeclaration(w io.Writer) {
	if t.publicTypeNameOverride != "" {
		fmt.Fprintf(w, "type %s %s\n", t.InternalName(), t.resolvedUnderlyingType.InternalName())
	}
}

func (t *baseType) TranslateToPublic(inputVar string) string {
	if t.translateToPublicOverride != "" {
		return fmt.Sprintf("%s(%s)", t.translateToPublicOverride, inputVar)
	} else {
		return t.simpleType.TranslateToPublic(inputVar)
	}
}

func (t *baseType) TranslateToInternal(inputVar string) string {
	if t.translateToInternalOverride != "" {
		return fmt.Sprintf("%s(%s)", t.translateToInternalOverride, inputVar)
	} else {
		return t.simpleType.TranslateToInternal(inputVar)
	}
}

func (t *baseType) PrintPublicToInternalTranslation(w io.Writer, inputVar, outputVar, _ string) {
	if t.translateToPublicOverride != "" {
		fmt.Fprintf(w, "%s := %s(%s)\n", outputVar, t.translateToInternalOverride, inputVar)
	} else {
		t.simpleType.PrintPublicToInternalTranslation(w, inputVar, outputVar, "")
	}
}

func (t *baseType) PrintTranslateToInternal(w io.Writer, inputVar, outputVar string) {
	if t.translateToInternalOverride != "" {
		fmt.Fprintf(w, "%s = %s(%s)", outputVar, t.translateToPublicOverride, inputVar)
	} else {
		t.simpleType.PrintTranslateToInternal(w, inputVar, outputVar)
	}
}

func ReadBaseTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, _ ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//types/type[@category='basetype']") {
		newType := NewBaseTypeFromXML(node)
		if tr[newType.RegistryName()] != nil {
			logrus.WithField("registry name", newType.RegistryName()).Warn("Overwriting base type in registry")
		}
		tr[newType.RegistryName()] = newType
	}
}

func NewBaseTypeFromXML(node *xmlquery.Node) TypeDefiner {
	rval := baseType{}
	rval.registryName = xmlquery.FindOne(node, "name").InnerText()

	typeNode := xmlquery.FindOne(node, "type")
	if typeNode == nil {
		rval.underlyingTypeName = "!empty_struct"
	} else {
		rval.underlyingTypeName = typeNode.InnerText()
	}

	// TODO: Count pointers here and handle appropriately
	rval.pointerDepth = strings.Count(node.InnerText(), "*")
	rval.underlyingTypeName = rval.underlyingTypeName + strings.Repeat("*", rval.pointerDepth)

	return &rval
}

func ReadBaseTypeExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("basetype").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
			return true
		} // Ignore comments

		entry := NewOrUpdateBaseTypeFromJSON(key.String(), exVal, tr, vr)
		tr[key.String()] = entry

		return true
	})

}
func NewOrUpdateBaseTypeFromJSON(key string, exception gjson.Result, tr TypeRegistry, vr ValueRegistry) TypeDefiner {
	existing := tr[key]
	var updatedEntry *baseType

	if existing == nil {
		logrus.WithField("registry type", key).Info("no existing registry entry for external type")
		updatedEntry = &baseType{}
		updatedEntry.registryName = key
	} else {
		updatedEntry = existing.(*baseType)
	}

	if utn := exception.Get("underlyingTypeName").String(); utn != "" {
		updatedEntry.underlyingTypeName = utn
	}

	updatedEntry.publicTypeNameOverride = exception.Get("go:type").String()
	updatedEntry.translateToPublicOverride = exception.Get("go:translatePublic").String()
	updatedEntry.translateToInternalOverride = exception.Get("go:translateInternal").String()

	updatedEntry.comment = exception.Get("comment").String()

	return updatedEntry
}

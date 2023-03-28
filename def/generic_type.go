package def

import (
	"fmt"
	"io"
)

type genericNamer struct {
	registryName, publicName, internalName string
}

func (t *genericNamer) RegistryName() string { return t.registryName }
func (t *genericNamer) PublicName() string   { return t.publicName }
func (t *genericNamer) InternalName() string { return t.internalName }

type genericType struct {
	genericNamer
	isResolved   bool
	comment      string
	pointerDepth int

	aliasTypeName     string
	resolvedAliasType TypeDefiner

	values []ValueDefiner
}

func (t *genericType) Category() TypeCategory { return CatNone }
func (t *genericType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	rval := NewIncludeSet()

	if t.publicName == "" {
		t.publicName = RenameIdentifier(t.registryName)
	}
	t.internalName = t.publicName

	if t.aliasTypeName != "" {
		t.resolvedAliasType = tr[t.aliasTypeName]
		rval.MergeWith(t.resolvedAliasType.Resolve(tr, vr))
	}

	rval.ResolvedTypes[t.registryName] = t
	return rval
}

func (t *genericType) IsIdenticalPublicAndInternal() bool { return true }

func (t *genericType) SetAliasType(td TypeDefiner) {
	t.aliasTypeName = td.RegistryName()
}

func (t *genericType) IsAlias() bool { return t.resolvedAliasType != nil }

func (t *genericType) AllValues() []ValueDefiner {
	return t.values
}
func (t *genericType) PushValue(vd ValueDefiner) {
	// Push will overwrite an existing name
	for i, v := range t.values {
		if v.RegistryName() == vd.RegistryName() {
			t.values[i] = vd
			return
		}
	}
	t.values = append(t.values, vd)
}

func (t *genericType) AppendValues(vals ValueRegistry) {
	for _, v := range vals {
		t.values = append(t.values, v)
	}
}

func (t *genericType) PrintGlobalDeclarations(io.Writer, int, bool) {}
func (t *genericType) PrintFileInitContent(io.Writer)               {}
func (t *genericType) RegisterImports(reg map[string]bool)          {}

func (t *genericType) PrintPublicDeclaration(w io.Writer) {
	fmt.Fprintln(w, "PrintPublicDeclaration not defined for genericType")
}

func (t *genericType) PrintInternalDeclaration(w io.Writer) {}

func (t *genericType) TranslateToPublic(inputVar string) string {
	return inputVar
}
func (t *genericType) TranslateToInternal(inputVar string) string {
	return inputVar
}

func (t *genericType) PrintPublicToInternalTranslation(w io.Writer, inputVar, outputVar, _ string) {
	fmt.Fprintf(w, "%s := %s\n", outputVar, inputVar)
}

func (t *genericType) PrintTranslateToInternal(w io.Writer, inputVar, outputVar string) {
	fmt.Fprintf(w, "%s = %s", outputVar, inputVar)
}

func (t *genericType) PrintDocLink(w io.Writer) {
	fmt.Fprintf(w, "// %s: ", t.PublicName())
	if t.comment != "" {
		fmt.Fprint(w, t.comment, "\n// ")
	}
	fmt.Fprintf(w, "See https://www.khronos.org/registry/vulkan/specs/1.3-extensions/man/html/%s.html\n", t.RegistryName())
}

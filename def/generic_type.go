package def

//go:generate stringer -type TypeCategory

import (
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

type Namer interface {
	RegistryName() string
	PublicName() string
	InternalName() string
}
type Resolver interface {
	Resolve(TypeRegistry, ValueRegistry) *includeSet
}

type TypeDefiner interface {
	Namer
	Resolver
	TypeAliaser

	IsIdenticalPublicAndInternal() bool

	Category() TypeCategory

	PushValue(ValueDefiner)
	AllValues() []ValueDefiner

	PrintGlobalDeclarations(io.Writer, int)
	PrintFileInitContent(io.Writer)

	PrintPublicDeclaration(io.Writer)
	PrintConstValues(io.Writer)

	PrintTranslateToInternal(w io.Writer, inputVar, outputVar string)
	PrintTranslateToPublic(w io.Writer, inputVar, outputVar string)
	PrintDocLink(w io.Writer)
}

type TypeAliaser interface {
	IsAlias() bool
	AliasType() TypeDefiner
	SetAliasType(TypeDefiner)
}

type ValueAliaser interface {
	IsAlias() bool
	AliasValue() ValueDefiner
}

type genericNamer struct {
	registryName, publicName, internalName string
}

type genericTypeResolver struct {
	isResolved        bool
	aliasRegistryName string
	resolvedAliasType TypeDefiner
}

type genericType struct {
	genericNamer
	genericTypeResolver

	comment string

	values    []ValueDefiner
	byteWidth int
}

func (t *genericNamer) RegistryName() string { return t.registryName }
func (t *genericNamer) PublicName() string   { return t.publicName }
func (t *genericNamer) InternalName() string { return t.internalName }

func (t *genericType) Category() TypeCategory { return CatGeneric }

func (t *genericType) IsIdenticalPublicAndInternal() bool { return true }

func (t *genericType) PrintTranslateToInternal(w io.Writer, inputVar, outputVar string) {
	panic("genericType cannot translate to internal representation")
}

func (t *genericType) PrintTranslateToPublic(w io.Writer, inputVar, outputVar string) {
	panic("genericType cannot translate to public representation")
}

func (t *genericType) IsAlias() bool          { return t.aliasRegistryName != "" }
func (t *genericType) AliasType() TypeDefiner { return t.resolvedAliasType }
func (t *genericType) SetAliasType(td TypeDefiner) {
	t.aliasRegistryName = td.RegistryName()
	t.resolvedAliasType = td
}

func (t *genericType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return nil
	}

	if t.aliasRegistryName != "" {
		t.resolvedAliasType = tr[t.aliasRegistryName]

		if t.resolvedAliasType == nil {
			logrus.WithField("registry name", t.registryName).
				WithField("alias name", t.aliasRegistryName).
				Error("alias not found in registry while resolving type")
			return nil
		} else {
			t.resolvedAliasType.Resolve(tr, vr)
			return &includeSet{
				includeTypeNames: []string{t.aliasRegistryName},
			}
		}
	}

	t.isResolved = true
	return nil
}

func (t *genericType) PushValue(val ValueDefiner) {
	t.values = append(t.values, val)
}
func (t *genericType) AllValues() []ValueDefiner {
	return t.values
}

func (t *genericType) PrintGlobalDeclarations(io.Writer, int) {}
func (t *genericType) PrintFileInitContent(io.Writer)         {}

func (t *genericType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)
	if t.comment != "" {
		fmt.Fprintln(w, "// ", t.comment)
	}

	fmt.Fprintf(w, "type %s = %s\n", t.PublicName(), t.resolvedAliasType.PublicName())
}

func (t *genericType) PrintConstValues(w io.Writer) {
	switch len(t.values) {
	case 0:
		return
	case 1:
		fmt.Fprintln(w)
		fmt.Fprint(w, "const ")
		t.values[0].PrintPublicDeclaration(w, true)
		fmt.Fprintln(w)
	default:
		fmt.Fprintln(w)
		fmt.Fprintln(w, "const (")
		for _, v := range t.values {
			v.PrintPublicDeclaration(w, !v.IsAlias())
		}
		fmt.Fprintln(w, ")")
		fmt.Fprintln(w)
	}
}

func (t *genericType) PrintDocLink(w io.Writer) {
	fmt.Fprintf(w, "// %s: See https://www.khronos.org/registry/vulkan/specs/1.3-extensions/man/html/%s.html\n",
		t.PublicName(), t.RegistryName(),
	)
}

type definedType struct {
	genericType
	underlyingTypeName     string
	resolvedUnderlyingType TypeDefiner
}

func (t *definedType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return nil
	}

	if t.underlyingTypeName == "" {
		return t.genericType.Resolve(tr, vr)
	} else {
		t.resolvedUnderlyingType = tr[t.underlyingTypeName]
		t.resolvedUnderlyingType.Resolve(tr, vr)

		t.isResolved = true
		return &includeSet{
			includeTypeNames: []string{t.underlyingTypeName},
		}
	}
}

func (t *definedType) PrintPublicDeclaration(w io.Writer) {
	t.PrintDocLink(w)

	if t.comment != "" {
		fmt.Fprintln(w, "// ", t.comment)
	}

	if t.IsAlias() {
		t.genericType.PrintPublicDeclaration(w)
	} else {
		fmt.Fprintf(w, "type %s %s\n", t.PublicName(), t.resolvedUnderlyingType.PublicName())
	}

	t.PrintConstValues(w)
}

func (t *definedType) PrintInternalDeclaration(w io.Writer) { /* NOP */ }

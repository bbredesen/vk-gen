package def

import (
	"fmt"
	"io"
	"strings"
)

// pointerType - Still a lot to do w/r/t slices vs strings vs "true" pointers
type pointerType struct {
	genericType

	resolvedPointsAtType TypeDefiner
	lenSpec              string

	externalSlice bool
}

func (t *pointerType) Category() TypeCategory { return CatPointer }

func (t *pointerType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {

	return t.resolvedPointsAtType.Resolve(tr, vr)
	// is := includeSet{
	// 	includeTypeNames: ,
	// }
}

func (t *pointerType) IsIdenticalPublicAndInternal() bool {
	// if this is a void pointer or if the underlying types are identical
	return t.resolvedPointsAtType.InternalName() == "!none" ||
		t.resolvedPointsAtType.InternalName() == "" ||
		(t.resolvedPointsAtType.IsIdenticalPublicAndInternal() && !t.isArrayPointer())
}

func (t *pointerType) isArrayPointer() bool {
	return t.lenSpec != ""
}

// PrintPublicDeclaration for a pointer type needs to determine if this pointer represents
// a remote array, a single value, or a fixed length array. There are several special cases,
// to handle void*, strings, fixed length arrays and slices.
func (t *pointerType) PublicName() string {
	registryName := t.resolvedPointsAtType.RegistryName()
	resolvedName := t.resolvedPointsAtType.PublicName()

	if resolvedName == "!none" || resolvedName == "" { // TODO WHY EMPTY STRING??
		return "unsafe.Pointer"
	} else if registryName == "char" {

		return "string"

	} else if t.lenSpec != "" {
		// If there is a length specifier, then this is an array, with char* -> string being a special case
		return "[]" + resolvedName
	} else {
		return "*" + resolvedName
	}
}

func (t *pointerType) InternalName() string {
	if t.resolvedPointsAtType.InternalName() == "!none" || t.resolvedPointsAtType.InternalName() == "" {
		return "unsafe.Pointer"
	}
	return "*" + t.resolvedPointsAtType.InternalName()
}

func (t *pointerType) TranslateToInternal(inputVar string) string {
	if t.lenSpec == "null-terminated" {
		return t.resolvedPointsAtType.TranslateToInternal(inputVar)
	} else if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
		return inputVar
	} else if t.resolvedPointsAtType.Category() == CatStruct || t.resolvedPointsAtType.Category() == CatUnion {
		return fmt.Sprintf("unsafe.Pointer(%s.Vulkanize())", inputVar)
	} else {
		return fmt.Sprintf("(%s)(%s)", t.InternalName(), t.resolvedPointsAtType.TranslateToInternal(inputVar))
	}
}

func (t *pointerType) PrintVulkanizeContent(forMember *structMember, preamble io.Writer) (structMemberAssignment string) {
	structMemberAssignment = "0 /* TODO POINTER NOT HANDLED */"

	if t.isArrayPointer() {
		if t.lenSpec == "null-terminated" {
			// Special case for strings, just give back the result of TranslateInternal
			structMemberAssignment = t.TranslateToInternal("s." + forMember.PublicName())
		} else if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
			// if this is an array to types that are "IsIdentical..." then just
			// assign the zero address and move on
			structMemberAssignment = t.TranslateToInternal(fmt.Sprintf("(&s.%s[0])", forMember.PublicName()))
		} else {
			pre := fmt.Sprintf(sliceTranslationTemplate,
				forMember.InternalName(), t.resolvedPointsAtType.InternalName(), forMember.PublicName(),
				forMember.PublicName(),
				forMember.InternalName(), t.resolvedPointsAtType.TranslateToInternal("v"),
			)
			fmt.Fprint(preamble, pre)
			structMemberAssignment = "&(sl_" + forMember.InternalName() + "[0])"
		}
	} else {
		if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
			structMemberAssignment = fmt.Sprintf("(%s)(s.%s)", t.InternalName(), forMember.PublicName())
		} else {
			structMemberAssignment = t.resolvedPointsAtType.TranslateToInternal("s." + forMember.PublicName())
			if t.resolvedPointsAtType.Category() == CatStruct {
				structMemberAssignment = strings.TrimLeft(structMemberAssignment, "*")
			}
		}
	}
	return
}

const sliceTranslationTemplate string = `
  sl_%s := make([]%s, len(s.%s))
  for i, v := range s.%s {
	sl_%s[i] = %s
  }
`

func (t *pointerType) PrintPublicToInternalTranslation(w io.Writer, publicValueName, internalValueName, internalLengthName string) {

	if t.resolvedPointsAtType.Category() == CatPointer {

		// Slice of pointers
		fmt.Fprintf(w, "sl_%s := make([]%s, len(%s))\n",
			internalValueName, t.resolvedPointsAtType.InternalName(), publicValueName)
		fmt.Fprintf(w, "for i, v:= range %s {\n", publicValueName)
		t.resolvedPointsAtType.PrintPublicToInternalTranslation(w, "v", "tmp", "")
		fmt.Fprintf(w, "  sl_%s[i] = tmp\n", internalValueName)
		fmt.Fprintf(w, "}\n")

		fmt.Fprintf(w, "%s := &sl_%s[0]\n", internalValueName, internalValueName)

		if t.lenSpec != "" {
			// if lenspec is empty this is one of the few altlen element
			fmt.Fprintf(w, "%s := uint32(len(sl_%s))\n", t.lenSpec, internalValueName)
		}

	} else if t.resolvedPointsAtType.RegistryName() == "char" {

		fmt.Fprintf(w, "%s := sys_stringToBytePointer(%s)\n", internalValueName, publicValueName)

	} else if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() && t.lenSpec == "" {
		fmt.Fprintf(w, "%s := (*%s)(%s)\n", internalValueName, t.resolvedPointsAtType.InternalName(), publicValueName)

	} else if internalLengthName != "" && internalLengthName != "null-terminated" {
		fmt.Fprintf(w, "  sl_%s := make([]%s, len(%s))\n", internalValueName, t.resolvedPointsAtType.InternalName(), publicValueName)

		fmt.Fprintf(w, "for i, v:= range %s {\n", publicValueName)
		if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
			fmt.Fprintf(w, "  sl_%s[i] = v\n", internalValueName)
		} else {
			fmt.Fprintf(w, "// WARNING TODO slice translation not handled\n")
			fmt.Fprintf(w, "_, _ = i, v\n")
		}
		fmt.Fprintln(w, "}")

		fmt.Fprintf(w, "%s := &sl_%s[0]\n", internalValueName, internalValueName)

		if t.lenSpec != "" {
			// if lenspec is empty this is one of the few altlen element
			fmt.Fprintf(w, "%s := uint32(len(sl_%s))\n", t.lenSpec, internalValueName)
		}

	} else if t.externalSlice {
		// if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
		// 	fmt.Fprintf(w, "%s = (*%s)(&%s[0])\n", internalValueName, t.resolvedPointsAtType.InternalName(), publicValueName)
		// 	fmt.Fprintf(w, "%s = len(%s)\n", internalLengthName, publicValueName)
		// } else {
		// 	// TODO: Need to translate each value in the slice...
		fmt.Fprintf(w, "// marked as external slice TODO Translate each item in the slice\n")
		// }
	} else if t.resolvedPointsAtType.Category() == CatStruct {
		fmt.Fprintf(w, "%s := %s.Vulkanize()\n", internalValueName, publicValueName)
	}

}

func (t *pointerType) PrintInternalToPublicTranslation(w io.Writer, internalLength int, internalValueName, publicValueName string) {
	if t.externalSlice {
		if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
			fmt.Fprintf(w,
				`var sl = struct {
					addr unsafe.Pointer
					len, cap  int
				}{unsafe.Pointer(%s), %d, %d}
				%s := (*(*[]%s)(unsafe.Pointer(&sl)))
				`,
				internalValueName,
				internalLength, internalLength,
				publicValueName,
				t.resolvedPointsAtType.PublicName(),
			)
		} else {
			fmt.Fprintf(w, "%s := make([]%s, %d)\n", publicValueName, t.resolvedPointsAtType.InternalName(), internalLength)
			fmt.Fprintf(w, "TODO: translate/copy values into slice\n")
		}
		// if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
		// 	fmt.Fprintf(w, "%s := make([]%s, %d)\n", publicValueName, t.resolvedPointsAtType.InternalName(), internalLength)
		// }
	}
}

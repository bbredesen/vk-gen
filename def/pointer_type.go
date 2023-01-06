package def

import (
	"fmt"
	"io"
)

// pointerType - Still a lot to do w/r/t slices vs strings vs "true" pointers
type pointerType struct {
	genericType

	resolvedPointsAtType TypeDefiner
	lenSpecString        string

	externalSlice bool
}

func (t *pointerType) Category() TypeCategory { return CatPointer }

func (t *pointerType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	return t.resolvedPointsAtType.Resolve(tr, vr)
	// is := includeSet{
	// 	includeTypeNames: ,
	// }
}

// PrintPublicDeclaration for a pointer type needs to determine if this pointer represents
// a remote array, a single value, or a fixed length array. There are several special cases,
// to handle void*, strings, fixed length arrays and slices.
func (t *pointerType) PublicName() string {
	registryName := t.resolvedPointsAtType.RegistryName()
	resolvedName := t.resolvedPointsAtType.PublicName()

	if resolvedName == "!none" {
		return "unsafe.Pointer"
	} else if registryName == "char" {
		return "string"
	} else if t.externalSlice {
		// If there is a length specifier, then this is an array, with char* -> string being a special case
		return "[]" + resolvedName
	} else {
		return "*" + resolvedName
	}
}

func (t *pointerType) InternalName() string {
	return "*" + t.resolvedPointsAtType.InternalName()
}

func (t *pointerType) PrintPublicToInternalTranslation(w io.Writer, publicValueName, internalValueName, internalLengthName string) {
	if t.externalSlice {
		if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
			fmt.Fprintf(w, "%s = (*%s)(&%s[0])\n", internalValueName, t.resolvedPointsAtType.InternalName(), publicValueName)
			fmt.Fprintf(w, "%s = len(%s)\n", internalLengthName, publicValueName)
		} else {
			// Need to translate each value in the slice...
		}
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

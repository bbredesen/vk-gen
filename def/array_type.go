package def

import (
	"fmt"
	"io"
)

// pointerType - Still a lot to do w/r/t slices vs strings vs "true" pointers
type arrayType struct {
	genericType

	resolvedPointsAtType TypeDefiner
	lenSpec              string
	// resolvedLenSpec      ValueDefiner
}

func (t *arrayType) Category() TypeCategory { return CatArray }

func (t *arrayType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {

	return t.resolvedPointsAtType.Resolve(tr, vr)
}

func (t *arrayType) IsIdenticalPublicAndInternal() bool {
	// if this is a void pointer or if the underlying types are identical
	return t.resolvedPointsAtType.InternalName() == "!none" ||
		t.resolvedPointsAtType.InternalName() == "" ||
		t.resolvedPointsAtType.IsIdenticalPublicAndInternal()
}

// PrintPublicDeclaration for an array still needs to handle strings. Will be a
// string on the public side and a fixed length char array on the internal side.
func (t *arrayType) PublicName() string {
	if t.resolvedPointsAtType.PublicName() == "byte" {
		return "string"
	}
	return fmt.Sprintf("[%s]%s", trimVk(t.lenSpec), t.resolvedPointsAtType.PublicName())
}

func (t *arrayType) InternalName() string {

	return fmt.Sprintf("[%s]%s", trimVk(t.lenSpec), t.resolvedPointsAtType.InternalName())
}

func (t *arrayType) TranslateToInternal(inputVar string) string {
	if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
		return inputVar
	} else {
		return fmt.Sprintf("(%s) /* NOT YET HANDLED ARRAY TYPE */", inputVar)
	}
}

func (t *arrayType) PrintVulkanizeContent(forMember *structMember, preamble io.Writer) (structMemberAssignment string) {
	structMemberAssignment = "0 /* TODO ARRAY NOT HANDLED */"

	if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
		// if this is an array to types that are "IsIdentical..." then just
		// copy the array as a block and move on
		fmt.Fprintf(preamble, "copy(rval.%s[:], s.%s)\n", forMember.InternalName(), forMember.PublicName())

		structMemberAssignment = ""
	}

	return
}
func (t *arrayType) PrintGoifyContent(forMember *structMember, preamble, epilogue io.Writer) (structMemberAssignment string) {

	if t.resolvedPointsAtType.InternalName() == "byte" {
		structMemberAssignment = fmt.Sprintf("nullTermBytesToString(s.%s[:])", forMember.InternalName())
		return
	}

	fmt.Fprintf(epilogue, "copy(rval.%s[:], s.%s[:])\n", forMember.PublicName(), forMember.InternalName())

	structMemberAssignment = ""
	return
}

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

func (t *arrayType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {

	return t.resolvedPointsAtType.Resolve(tr, vr)
	// is := includeSet{
	// 	includeTypeNames: ,
	// }
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
	} else {
		// 		pre := fmt.Sprintf(sliceTranslationTemplate,
		// 			forMember.InternalName(), t.resolvedPointsAtType.InternalName(), forMember.PublicName(),
		// 			forMember.PublicName(),
		// 			forMember.InternalName(), t.resolvedPointsAtType.TranslateToInternal("v"),
		// 		)
		// 		fmt.Fprint(preamble, pre)
		// 		structMemberAssignment = "&(sl_" + forMember.InternalName() + "[0])"
		// 	}
		// } else {
		// 	if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
		// 		structMemberAssignment = fmt.Sprintf("(%s)(s.%s)", t.InternalName(), forMember.PublicName())
		// 	} else {
		// 		structMemberAssignment = t.resolvedPointsAtType.TranslateToInternal("s." + forMember.PublicName())
		// 		if t.resolvedPointsAtType.Category() == CatStruct {
		// 			structMemberAssignment = strings.TrimLeft(structMemberAssignment, "*")
		// 		}
		// 	}
	}
	return
}
func (t *arrayType) PrintGoifyContent(forMember *structMember, preamble, epilogue io.Writer) (structMemberAssignment string) {
	structMemberAssignment = "0 /* TODO ARRAY NOT HANDLED */"

	// if t.resolvedPointsAtType.IsIdenticalPublicAndInternal() {
	// if this is an array to types that are "IsIdentical..." then just
	// copy the array as a block and move on
	fmt.Fprintf(epilogue, "copy(rval.%s[:], s.%s[:])\n", forMember.PublicName(), forMember.InternalName())

	structMemberAssignment = ""
	// } else {
	// 	structMemberAssignment = t.resolvedPointsAtType.TranslateToPublic("s." + forMember.InternalName())
	// }
	return
}

// const sliceTranslationTemplate string = `
//   sl_%s := make([]%s, len(s.%s))
//   for i, v := range s.%s {
// 	sl_%s[i] = %s
//   }
// `

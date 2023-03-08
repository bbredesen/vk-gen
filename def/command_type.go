package def

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/antchfx/xmlquery"
	"github.com/iancoleman/strcase"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type commandType struct {
	genericType
	returnTypeName     string
	resolvedReturnType TypeDefiner

	staticCodeRef string

	parameters []*commandParam

	bindingParams []*commandParam
	returnParams  []*commandParam
	// identicalInternalExternal bool
	// isReturnedOnly            bool
	bindingParamCount int
}

// Two exceptions to camelCase rules used for function return params
func init() {
	// rename return params to avoid typenames
	strcase.ConfigureAcronym("Result", "r")
	strcase.ConfigureAcronym("PFN_vkVoidFunction", "fn")
	strcase.ConfigureAcronym("uint32", "r")
	strcase.ConfigureAcronym("uint64", "r")
	strcase.ConfigureAcronym("bool", "r")
}

func (t *commandType) findParam(regName string) *commandParam {
	for _, p := range t.parameters {
		if p.registryName == regName {
			return p
		}
	}
	return nil
}

func (t *commandType) keyName() string        { return "key" + t.registryName }
func (t *commandType) Category() TypeCategory { return CatCommand }

func (t *commandType) IsIdenticalPublicAndInternal() bool { return true }

func (t *commandType) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if t.isResolved {
		return NewIncludeSet()
	}

	iset := t.genericType.Resolve(tr, vr)

	if !t.IsAlias() && t.returnTypeName != "" {

		t.resolvedReturnType = tr[t.returnTypeName]
		if t.resolvedReturnType == nil {
			log.WithField("registry name", t.registryName).
				WithField("return registry name", t.returnTypeName).
				Error("return type was not found while resolving command")
		} else {
			iset.MergeWith(t.resolvedReturnType.Resolve(tr, vr))

		}
	}

	// if t.publicName == "" { // publicName may already be set by an entry from exceptions.json
	// 	t.publicName = RenameIdentifier(t.registryName)
	// }

	for _, p := range t.parameters {
		p.parentCommand = t

		iset.MergeWith(p.Resolve(tr, vr))
	}

	// t.identicalInternalExternal = t.determineIdentical()

	iset.ResolvedTypes[t.registryName] = t

	t.isResolved = true
	return iset
}

func (t *commandType) PrintGlobalDeclarations(w io.Writer, idx int, isStart bool) {
	if t.staticCodeRef != "" || t.IsAlias() {
		// Ignored, static refs from exceptions.json aren't processed through
		// Cgo/lazyCommands and no need to write key entries for alias commands
		return
	}

	if isStart {
		if idx == 0 {
			fmt.Fprintf(w, "%s vkCommandKey = iota\n", t.keyName())
		} else {
			fmt.Fprintf(w, "%s vkCommandKey = iota + %d\n", t.keyName(), idx)
		}
	} else {
		fmt.Fprintln(w, t.keyName())
	}
}

func (t *commandType) PrintFileInitContent(w io.Writer) {
	if t.IsAlias() {
		// No need to write key entries for alias commands
		return
	}
	fmt.Fprintf(w, "lazyCommands[%s] = vkCommand{\"%s\", %d, %v, nil}\n",
		t.keyName(), t.RegistryName(), t.bindingParamCount, t.resolvedReturnType != nil)

}

func (t *commandType) PrintPublicDeclaration(w io.Writer) {
	if t.staticCodeRef != "" {
		fmt.Fprintf(w, "// %s is static code, not generated from vk.xml; aliased to %s\n", t.PublicName(), t.staticCodeRef)
		fmt.Fprintf(w, "var %s = %s\n\n", t.PublicName(), t.staticCodeRef)
		return
	} else if t.IsAlias() {
		fmt.Fprintf(w, "var %s = %s\n\n", t.PublicName(), t.resolvedAliasType.PublicName())
		return
	}
	// funcReturnNames, funcReturnTypes := &strings.Builder{}, &strings.Builder{}

	preamble, epilogue, outputTranslation := &strings.Builder{}, &strings.Builder{}, &strings.Builder{}

	// var doubleCallArrayParam *commandParam
	// var doubleCallArrayTypeName string

	// var isDoubleCallCommand bool

	// for _, p := range t.bindingParams {
	// 	funcParams = funcParams + ", " + p.publicName + " " + p.resolvedType.PublicName()
	// }

	funcReturnParams := make([]*commandParam, 0)
	var trampolineReturns *commandParam
	funcInputParams := make([]*commandParam, 0)
	funcTrampolineParams := make([]*commandParam, 0)

	if t.resolvedReturnType.RegistryName() != "void" {
		retParam := &commandParam{}
		retParam.resolvedType = t.resolvedReturnType
		retParam.publicName = strcase.ToLowerCamel(t.resolvedReturnType.PublicName())
		funcReturnParams = append(funcReturnParams, retParam)
		trampolineReturns = retParam
	}

	// Start with a simple output scenario - vkEndCommandBuffer takes a single
	// input (ignore for the moment) and returns a VkResult
	// Deal with simple inputs, like handles and primitive/scalar types
	// Convert inputs that require translation
	// Deal with simple outputs through pointers - e.g. GetBufferMemoryRequirements
	// Deal with slice inputs - eg WaitForFences - fenceCount, pFences

	for _, p := range t.parameters {
		// After classification, the params need to be in one or more of:
		// - Go function parameters
		// - Go return values
		// - Trampoline parameters, with or without translation

		// if p.isDoubleCallArray {
		// 	isDoubleCallCommand = true
		// 	// This gets saved for later printing the calls
		// 	doubleCallArrayParam = p

		// 	fmt.Fprintf(funcReturnNames, ", %s", p.publicName)
		// 	retSliceType := p.resolvedType.(*pointerType).resolvedPointsAtType
		// 	fmt.Fprintf(funcReturnTypes, "[]%s", retSliceType.PublicName())
		// } else if p.isOutput {
		// 	fmt.Fprintf(funcReturnNames, ", %s", p.publicName)
		// 	fmt.Fprintf(funcReturnTypes, ", %s", p.resolvedType.PublicName())
		// } else {
		// 	fmt.Fprintf(funcParams, ", %s %s", p.publicName, p.resolvedType.PublicName())
		// }
		if p.resolvedType.Category() == CatPointer {
			paramTypeAsPointer := p.resolvedType.(*pointerType)

			if p.isConstParam {
				funcInputParams = append(funcInputParams, p)

				if p.lenMemberParam != nil {
					// Parameter is an input array/slice

					if p.requiresTranslation {
						fmt.Fprintf(preamble, "  // %s is an input slice that requires translation to an internal type\n", p.publicName)
						fmt.Fprintf(preamble, "  var %s unsafe.Pointer\n", p.internalName)
						fmt.Fprintf(preamble, "  if len(%s) > 0 {\n", p.publicName)
						fmt.Fprintf(preamble, "    sl_%s := make([]%s, %s)\n", p.publicName, paramTypeAsPointer.resolvedPointsAtType.InternalName(), p.lenMemberParam.publicName)
						fmt.Fprintf(preamble, "    for i, v := range %s {\n", p.publicName)
						fmt.Fprintf(preamble, "      sl_%s[i] = %s\n", p.publicName, paramTypeAsPointer.resolvedPointsAtType.TranslateToInternal("v"))
						fmt.Fprintf(preamble, "    }\n")
						fmt.Fprintf(preamble, "    %s = unsafe.Pointer(&sl_%s[0])\n", p.internalName, p.publicName)
						fmt.Fprintf(preamble, "  }\n")
						fmt.Fprintln(preamble)

						funcTrampolineParams = append(funcTrampolineParams, p)

					} else {
						// Parameter can be directly used (once we get a pointer
						// to the first element)
						fmt.Fprintf(preamble, "  // %s is an input slice of values that do not need translation used\n", p.publicName)
						fmt.Fprintf(preamble, "  var %s unsafe.Pointer\n", p.internalName)
						fmt.Fprintf(preamble, "  if %s != nil {\n", p.publicName)
						fmt.Fprintf(preamble, "    %s = unsafe.Pointer(&%s[0])\n", p.internalName, p.publicName)
						fmt.Fprintf(preamble, "  }\n")
						fmt.Fprintln(preamble)
						funcTrampolineParams = append(funcTrampolineParams, p)
					}

				} else if strings.Contains(p.lenSpec, "->") {
					// This is an edge case where the length of the return array (slice) is embedded in a struct, which
					// is another parameter. See vkGetAccelerationStructureBuildSizesKHR for an example (perhaps the
					// only example).
					lenparts := strings.Split(p.lenSpec, "->")
					otherParamInternalName := lenparts[0]
					// otherParamMemberName := lenparts[1]

					fmt.Fprintf(preamble, "  // %s is an input slice that requires translation to an internal type; length is embedded in %s\n", p.publicName, otherParamInternalName)
					fmt.Fprintf(preamble, "  %s := unsafe.Pointer(nil)\n", p.internalName) //, otherParamInternalName, otherParamMemberName)
					fmt.Fprintf(preamble, "  // WARNING TODO - passing nil pointer to get to a version that will compile. THIS VULKAN CALL WILL FAIL!")

					funcTrampolineParams = append(funcTrampolineParams, p)

				} else if p.altLenSpec != "" {
					// This is another edge case where the length of the array is embedded in a bitfield. For now, the
					// user must provide the bitfield and must ensure that their slice is the appropriate length.
					// See vkCmdSetSampleMaskEXT for an example. Addressed as "no action" relative to other slices in
					// Resolve(). Fix for issue #17
					fmt.Fprintf(preamble, "  // %s is an edge case input slice, with an alternative length encoding. Developer must provide the length themselves.\n", p.publicName)
					fmt.Fprintf(preamble, "  // No handling for internal vs. external types at this time, the only case this appears as of 1.3.240 is a handle type with a bitfield length encoding\n")
					fmt.Fprintf(preamble, "  var %s *%s\n", p.internalName, p.resolvedType.(*pointerType).resolvedPointsAtType.PublicName())
					fmt.Fprintf(preamble, "  if %s != nil {\n", p.publicName)
					fmt.Fprintf(preamble, "    %s = &%s[0]\n", p.internalName, p.publicName)
					fmt.Fprintf(preamble, "  }\n")

					funcTrampolineParams = append(funcTrampolineParams, p)

				} else {
					// Parameter is a singular input
					if p.resolvedType.IsIdenticalPublicAndInternal() {
						fmt.Fprintf(preamble, "// Parameter is a singular input, pass direct - %s\n", p.publicName)
						fmt.Fprintf(preamble, "  var %s unsafe.Pointer\n", p.internalName)
						fmt.Fprintf(preamble, "  if %s != nil {\n", p.publicName)
						fmt.Fprintf(preamble, "    %s = unsafe.Pointer(%s)\n", p.internalName, p.publicName)
						fmt.Fprintf(preamble, "  }\n")
						fmt.Fprintln(preamble)
						funcTrampolineParams = append(funcTrampolineParams, p)

					} else {
						fmt.Fprintf(preamble, "// Parameter is a singular input, requires translation - %s\n", p.publicName)
						// Special handling for strings, which come in as "" instead of nil
						nullValue := "nil"
						if p.resolvedType.PublicName() == "string" {
							// Vulkan accepts NULL or an empty string as the same value
							nullValue = `""`
						}

						fmt.Fprintf(preamble, "  var %s %s\n", p.internalName, p.resolvedType.InternalName())
						fmt.Fprintf(preamble, "  if %s != %s {\n", p.publicName, nullValue)
						fmt.Fprintf(preamble, "    %s = %s\n", p.internalName, p.resolvedType.TranslateToInternal(p.publicName))
						fmt.Fprintf(preamble, "  }\n")
						fmt.Fprintln(preamble)
						funcTrampolineParams = append(funcTrampolineParams, p)

					}
				}
			} else {
				if p.lenMemberParam != nil { // lenSpec != "": spec might be a reference into another input, e.g. vkAllocateCommandBuffers, pAllocateInfo->commandBufferCount
					// Parameter is an output
					if p.lenMemberParam.resolvedType.Category() == CatPointer {
						if p.lenMemberParam.isLenMemberFor[0] == p {

							// Parameter is a double-call array output
							fmt.Fprintf(preamble, "// %s is a double-call array output\n", p.publicName)
							// Allocate the length param and stub the slice
							fmt.Fprintf(preamble, "  var %s %s\n", p.lenMemberParam.publicName, p.lenMemberParam.resolvedType.(*pointerType).resolvedPointsAtType.PublicName())
							fmt.Fprintf(preamble, "  %s := &%s\n", p.lenMemberParam.internalName, p.lenMemberParam.publicName)

							fmt.Fprintf(preamble, "// first trampoline happens here; also, still need to check returned Result value\n")
							funcTrampolineParams = append(funcTrampolineParams, p.lenMemberParam)

						}

						// Need distinction between identical interal/external types and those that need to be
						// translated
						if p.resolvedType.IsIdenticalPublicAndInternal() {
							fmt.Fprintf(preamble, "// Identical internal and external")
							// fmt.Fprintf(preamble, "  var %s %s\n", p.publicName, p.resolvedType.PublicName())
							fmt.Fprintf(epilogue, "  %s = make([]%s, %s)\n", p.publicName, p.resolvedType.PublicName(), p.lenMemberParam.publicName)
							fmt.Fprintf(epilogue, "  %s := &%s[0]\n", p.internalName, p.publicName)
							fmt.Fprintln(epilogue)

						} else {
							fmt.Fprintf(preamble, "// NOT identical internal and external, result needs translation\n")
							fmt.Fprintf(preamble, "  var %s %s\n", p.internalName, p.resolvedType.InternalName())
							fmt.Fprintf(epilogue, "  sl_%s := make([]%s, %s)\n", p.internalName, p.resolvedType.(*pointerType).resolvedPointsAtType.InternalName(), p.lenMemberParam.publicName)
							fmt.Fprintf(epilogue, "  %s = make(%s, %s)\n", p.publicName, p.resolvedType.PublicName(), p.lenMemberParam.publicName)
							fmt.Fprintf(epilogue, "  %s = &sl_%s[0]\n", p.internalName, p.internalName)
							fmt.Fprintln(epilogue)

							fmt.Fprintf(outputTranslation,
								`for i := range sl_%s {
	%s[i] = *%s
}
`, p.internalName, p.publicName, p.resolvedType.TranslateToPublic("sl_"+p.internalName+"[i]"))
						}

						funcTrampolineParams = append(funcTrampolineParams, p)
						funcReturnParams = append(funcReturnParams, p)

						if p.lenMemberParam.isLenMemberFor[len(p.lenMemberParam.isLenMemberFor)-1] == p {
							// If there is more than one array to allocate, make sure we only call trampoline on the last one
							fmt.Fprintf(epilogue, "// Trampoline call after last array allocation\n")
							t.printTrampolineCall(epilogue, funcTrampolineParams, trampolineReturns)
							fmt.Fprintln(epilogue)

							// If the output requires translation, iterate the slice and translate here
							fmt.Fprintf(epilogue, outputTranslation.String())

						}

					} else if p.lenMemberParam != nil {
						// If the length parameter references a const param in addition to
						// this one (which we have already established is not
						// const), then the length is derived from the length of
						// the the other param, which will be an input slice.
						//
						// Therefore, this will be an output allocated by
						// the binding based on the length of the other slice.
						//
						// Otherwise, this parameter must be pre-allocated by
						// the user? Or it must be an input?

						// Check for other const param in p.lenMemberParam
						allocForOutput := false
						for _, q := range p.lenMemberParam.isLenMemberFor {
							allocForOutput = allocForOutput || q.isConstParam
						}

						if allocForOutput {
							fmt.Fprintf(preamble, "// %s is an output array that will be allocated by the binding, len is from %s\n", p.publicName, p.lenMemberParam.publicName)
							fmt.Fprintf(preamble, "  %s = make([]%s, %s)\n", p.publicName, paramTypeAsPointer.resolvedPointsAtType.PublicName(), p.lenMemberParam.publicName)
							fmt.Fprintf(preamble, "  %s := unsafe.Pointer(&%s[0])\n", p.internalName, p.publicName)
							fmt.Fprintln(preamble)

							funcTrampolineParams = append(funcTrampolineParams, p)
							funcReturnParams = append(funcReturnParams, p)

						} else {
							fmt.Fprintf(preamble, "// %s is a user-allocated array input that will be written to\n", p.publicName)
							// e.g., vkGetQueryPoolResults matches this rule
							// I just want to accept a []byte, extract the
							// length and push len+pointer to trampoline
							// fmt.Fprintf(preamble, "  %s := len(%s)\n", p.lenMemberParam.publicName, p.publicName)
							fmt.Fprintf(preamble, "  %s := unsafe.Pointer(&%s[0])\n", p.internalName, p.publicName)
							fmt.Fprintln(preamble)

							funcInputParams = append(funcInputParams, p)
							funcTrampolineParams = append(funcTrampolineParams, p)
						}

					}
				} else if p.isLenMemberFor != nil {
					p.isOutput = false

				} else {
					// Parameter is user allocated but will be populated by Vulkan. Binding will treat it as an output only.
					if p.lenSpec != "" {
						if p.lenMemberParam == nil {
							fmt.Fprintf(preamble, "// Parameter is binding-allocated array populated by Vulkan; length is possibly embedded in a struct (%s) - %s\n", p.lenSpec, p.publicName)
							stringParts := strings.Split(p.lenSpec, "->")
							translatedLenMember := stringParts[0] + "." + stringParts[1]

							// fmt.Fprintf(preamble, "  sl_%s := make([]%s, %s)\n", p.internalName, p.resolvedType.InternalName(), translatedLenMember)
							// fmt.Fprintf(preamble, "  %s := &sl_%s[0]\n", p.internalName, p.internalName)
							fmt.Fprintf(preamble, "  %s = make(%s, %s)\n", p.publicName, p.resolvedType.PublicName(), translatedLenMember)
							fmt.Fprintf(preamble, "  %s := &%s[0]\n", p.internalName, p.publicName)

							// At a practical level, this is only used to return an array of handles, we can avoid translation altogether; see
							// AllocateCommandBuffers for an example. It is possible that a future API release will need
							// updates here.

						} else {
							fmt.Fprintf(preamble, "// Parameter is binding-allocated array populated by Vulkan; length is provided by what? (%s) - %s\n", p.publicName, p.lenSpec)
							panic("Parameter is binding-allocated array populated by Vulkan, length not provided?? " + p.lenSpec)
						}

						// rewrite the return param as a slice
						derefedReturnParam := *p
						// derefedReturnParam.resolvedType = paramTypeAsPointer.resolvedPointsAtType

						funcTrampolineParams = append(funcTrampolineParams, p)
						funcReturnParams = append(funcReturnParams, &derefedReturnParam)
					} else {
						if p.resolvedType.IsIdenticalPublicAndInternal() {
							fmt.Fprintf(preamble, "// %s is a binding-allocated single return value and will be populated by Vulkan\n", p.publicName)

							p.internalName = "ptr_" + p.internalName // This should really be done in resolve, not here

							fmt.Fprintf(preamble, "  %s := &%s\n", p.internalName, p.publicName)
							fmt.Fprintln(preamble)
						} else {
							fmt.Fprintf(preamble, "// %s is a binding-allocated single return value and will be populated by Vulkan, but requiring translation\n", p.publicName)
							if p.resolvedType.Category() == CatPointer {
								fmt.Fprintf(preamble, "var %s %s\n", p.internalName, p.resolvedType.(*pointerType).resolvedPointsAtType.InternalName())
								fmt.Fprintf(preamble, "ptr_%s := &%s\n", p.internalName, p.internalName)

								fmt.Fprintf(epilogue, "  %s = %s\n", p.publicName, p.resolvedType.(*pointerType).resolvedPointsAtType.TranslateToPublic(p.internalName))
							} else {
								fmt.Fprintf(preamble, "var %s %s\n", p.internalName, p.resolvedType.InternalName())
								fmt.Fprintf(preamble, "ptr_%s := &%s\n", p.internalName, p.internalName)

								fmt.Fprintf(epilogue, "  %s = *%s\n", p.publicName, p.resolvedType.TranslateToPublic(p.internalName))
							}

							p.internalName = "ptr_" + p.internalName // Done after the fact so the internal pointer will be used in the tramp params

							fmt.Fprintln(preamble)

						}

						// rewrite the return param as not a pointer
						derefedReturnParam := *p
						derefedReturnParam.resolvedType = paramTypeAsPointer.resolvedPointsAtType

						funcTrampolineParams = append(funcTrampolineParams, p)
						funcReturnParams = append(funcReturnParams, &derefedReturnParam)
					}
				}
			}
		} else {
			// Non-pointer parameters
			if p.isLenMemberFor != nil {
				// A non-optional length parameter is the length of an input array
				if !p.isAlwaysOptional && p.resolvedType.PublicName() == "unsafe.Pointer" {
					funcInputParams = append(funcInputParams, p)
				} else {
					fmt.Fprintf(preamble, "%s := len(%s)\n", p.publicName, p.isLenMemberFor[0].publicName)
				}

				p.isInput = false
				p.isOutput = false

				funcTrampolineParams = append(funcTrampolineParams, p)
			}

			if p.requiresTranslation {
				// Non-pointer types have the same name for internal and public,
				// but we would attempt redefine that variable here. Postfix the
				// param name with the internal type to avoid the conflict. See
				// vkWaitForFences for an example involving Bool32
				p.internalName = p.internalName + "_" + p.resolvedType.InternalName()
				fmt.Fprintf(preamble, "%s := %s\n", p.internalName, p.resolvedType.TranslateToInternal(p.publicName))
			}

			if p.isOutput {
				funcReturnParams = append(funcReturnParams, p)
				funcTrampolineParams = append(funcTrampolineParams, p)
			}
			if p.isInput {
				funcInputParams = append(funcInputParams, p)
				funcTrampolineParams = append(funcTrampolineParams, p)
			}
		}

	}

	specStringFromParams := func(sl []*commandParam) string {
		sb := &strings.Builder{}
		for _, param := range sl {
			fmt.Fprintf(sb, ", %s %s", param.publicName, param.resolvedType.PublicName())
		}
		return strings.TrimPrefix(sb.String(), ", ")
	}

	t.bindingParamCount = len(funcTrampolineParams)

	inputSpecString, returnSpecString := specStringFromParams(funcInputParams), specStringFromParams(funcReturnParams)

	t.PrintDocLink(w)
	fmt.Fprintf(w, "func %s(%s) (%s) {\n",
		t.PublicName(),
		inputSpecString,
		returnSpecString)

	fmt.Fprintln(w, preamble.String())

	t.printTrampolineCall(w, funcTrampolineParams, trampolineReturns)
	fmt.Fprintln(w)

	fmt.Fprintf(w, epilogue.String())

	if len(funcReturnParams) > 0 {
		fmt.Fprintf(w, "  return\n")
	}

	fmt.Fprintf(w, "}\n\n")
}

func trampStringFromParams(sl []*commandParam) string {
	sb := &strings.Builder{}
	for _, param := range sl {
		if param.resolvedType.Category() == CatPointer {
			fmt.Fprintf(sb, ", uintptr(unsafe.Pointer(%s))", param.internalName)
		} else {
			fmt.Fprintf(sb, ", uintptr(%s)", param.internalName)
		}
	}
	// Note that the leading ", " is not trimmed
	return sb.String()
}

func (t *commandType) printTrampolineCall(w io.Writer, trampParams []*commandParam, returnParam *commandParam) {
	trampParamsString := trampStringFromParams(trampParams)

	if returnParam != nil {
		if returnParam.resolvedType.IsIdenticalPublicAndInternal() {
			fmt.Fprintf(w, "  %s = %s(execTrampoline(%s%s))\n", returnParam.publicName, returnParam.resolvedType.PublicName(), t.keyName(), trampParamsString)
		} else {
			fmt.Fprintf(w, "  rval := %s(execTrampoline(%s%s))\n", returnParam.resolvedType.InternalName(), t.keyName(), trampParamsString)
			fmt.Fprintf(w, "  %s = %s\n", returnParam.publicName, returnParam.resolvedType.TranslateToPublic("rval"))
		}
	} else {
		fmt.Fprintf(w, "  execTrampoline(%s%s)\n", t.keyName(), trampParamsString)
	}
}

func (t *commandType) PrintInternalDeclaration(w io.Writer) {

	// var preamble, structDecl, epilogue strings.Builder

	// if t.identicalInternalExternal {
	// 	fmt.Fprintf(w, "type %s = %s\n", t.InternalName(), t.PublicName())
	// } else {
	// 	if t.isReturnedOnly {
	// 		fmt.Fprintf(w, "// WARNING - This struct is returned only, which is not yet handled in the binding\n")
	// 	}
	// 	// _vk type declaration
	// 	fmt.Fprintf(w, "type %s struct {\n", t.InternalName())
	// 	for _, m := range t.members {
	// 		m.PrintInternalDeclaration(w)
	// 	}

	// 	fmt.Fprintf(w, "}\n")
	// }

	// // Vulkanize declaration
	// // Set required values, like the stype
	// // Expand slices to pointer and length parameters
	// // Convert strings, and string arrays
	// fmt.Fprintf(&preamble, "func (s *%s) Vulkanize() *%s {\n", t.PublicName(), t.InternalName())

	// if t.identicalInternalExternal {
	// 	fmt.Fprintf(&structDecl, "  rval := %s(*s)\n", t.InternalName())
	// } else {
	// 	fmt.Fprintf(&structDecl, "  rval := %s{\n", t.InternalName())
	// 	for _, m := range t.members {
	// 		m.PrintVulcDeclarationAsssignment(&preamble, &structDecl, &epilogue)
	// 	}
	// 	fmt.Fprintf(&structDecl, "  }\n")
	// }
	// fmt.Fprintf(&epilogue, "  return &rval\n")
	// fmt.Fprintf(&epilogue, "}\n")

	// fmt.Fprint(w, preamble.String(), structDecl.String(), epilogue.String())

	// Goify declaration (if applicable?)
}

type commandParam struct {
	registryName string
	typeName     string
	resolvedType TypeDefiner

	publicName, internalName string

	isConstParam        bool
	optionalParamString string
	isAlwaysOptional    bool

	pointerLevel        int
	lenSpec, altLenSpec string

	parentCommand  *commandType
	isResolved     bool
	isLenMemberFor []*commandParam
	lenMemberParam *commandParam

	isInput, isOutput                          bool
	isPublicSlice                              bool
	isOutputArray                              bool
	isDoubleCallArray, isDoubleCallArrayLength bool
	requiresTranslation                        bool
}

func (p *commandParam) Resolve(tr TypeRegistry, vr ValueRegistry) *IncludeSet {
	if p.isResolved {
		return NewIncludeSet()
	}

	iset := NewIncludeSet()
	iset.IncludeTypes[p.typeName] = true

	p.resolvedType = tr[p.typeName]
	iset.MergeWith(p.resolvedType.Resolve(tr, vr))

	// Build the pointer chain if applicable
	for i := 0; i < p.pointerLevel; i++ {
		ptr := pointerType{}
		ptr.resolvedPointsAtType = p.resolvedType
		p.resolvedType = &ptr
	}

	// check for length specification
	if p.altLenSpec != "" {
		// If altlen is present, then the array is a fixed length per the spec.
		// as of 1.3.240, only vkCmdSetSampleMaskEXT has an altlen parameter, where the expected array length is
		// embedded in a sample mask bitfield.
		//
		// Fix for issue #17 is to recognize this parameter as a slice, but we won't try to calculate the bitfield.
		// (i.e., the developer needs to just pass a SampleMaskBits value that matches the slice)
		//
		// Net effect is to do noting here in Resolve.

	} else if p.lenSpec != "" {
		for _, otherP := range p.parentCommand.parameters {
			if otherP.registryName == p.lenSpec {
				otherP.isLenMemberFor = append(otherP.isLenMemberFor, p)
				p.lenMemberParam = otherP
				break
			}
		}
	}

	// if this param is undecorated, is not a pointer, and is not the length
	// for another param, it is just straight input to pass through

	p.requiresTranslation = !p.resolvedType.IsIdenticalPublicAndInternal()

	if p.resolvedType.Category() == CatPointer {
		resTypeAsPointer := p.resolvedType.(*pointerType)
		resTypeAsPointer.lenSpec = p.lenSpec

		// if this param is an undecorated const pointer, it is input of a
		// struct.
		if p.isConstParam {
			if p.lenSpec == "" {
				p.isInput = true
				// p.requiresTranslation = !p.resolvedType.IsIdenticalPublicAndInternal()

			} else if p.lenMemberParam != nil {
				// if this param is a const pointer with a len specifier that maps to
				// another parameter name, then this is an input array, represented as a
				// slice on the public side
				p.isInput = true
				p.isPublicSlice = true
			}
		} else {
			// if this param is a non-const pointer with a len specifier that maps
			// to another parameter name, then this is a double-call output array
			// EXCEPT when that len param is also for an input array. Then this is a
			// single-call output array
			if p.lenMemberParam != nil {
				if len(p.lenMemberParam.isLenMemberFor) > 1 {
					p.isOutputArray = true
					p.isPublicSlice = true
				} else {
					p.isDoubleCallArray = true
					p.isPublicSlice = true
				}
			} else if len(p.isLenMemberFor) > 0 {
				// if this param is a non-const pointer that is a len specifier for
				// another parameter, then this is a double-call array length
				p.isDoubleCallArrayLength = true
			}
		}
	} else {
		p.isInput = true
	}

	p.internalName = RenameIdentifier(p.registryName)

	if p.resolvedType.Category() == CatPointer {
		p.publicName = strings.TrimPrefix(RenameIdentifier(p.registryName), "p")
	} else {
		p.publicName = RenameIdentifier(p.registryName)
	}

	if p.publicName != "" {
		// Why this call? Shouldn't be any invalid UTF8 in the spec? Is this just to lower-case the string? Can we not
		// just call strcase.ToLowerCamel()?
		r, n := utf8.DecodeRuneInString(p.publicName)
		p.publicName = string(unicode.ToLower(r)) + p.publicName[n:]
	}

	p.isAlwaysOptional = p.optionalParamString == "true"

	if p.isAlwaysOptional || p.isConstParam || p.pointerLevel == 0 {
		p.parentCommand.bindingParams = append(p.parentCommand.bindingParams, p)
	} else {
		p.parentCommand.returnParams = append(p.parentCommand.returnParams, p)
	}

	p.isResolved = true

	return iset
}

func ReadCommandTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) {
	for _, commandNode := range append(xmlquery.Find(doc, "//commands/command"), xmlquery.Find(doc, "//extension/command")...) {
		val := NewCommandFromXML(commandNode)
		tr[val.RegistryName()] = val
	}
}

func NewCommandFromXML(elt *xmlquery.Node) *commandType {
	rval := commandType{}
	name := elt.SelectAttr("name")
	if name != "" {
		rval.registryName = name
		rval.aliasTypeName = elt.SelectAttr("alias")
	} else {
		rval.registryName = xmlquery.FindOne(elt, "/proto/name").InnerText()
		rval.returnTypeName = xmlquery.FindOne(elt, "/proto/type").InnerText()

		for _, m := range xmlquery.Find(elt, "param") {
			par := NewCommandParamFromXML(m, &rval)
			rval.parameters = append(rval.parameters, par)
		}
	}

	rval.comment = elt.SelectAttr("comment")

	return &rval
}

func NewCommandParamFromXML(elt *xmlquery.Node, forCommand *commandType) *commandParam {
	rval := commandParam{}
	rval.registryName = xmlquery.FindOne(elt, "name").InnerText()
	rval.typeName = xmlquery.FindOne(elt, "type").InnerText()

	rval.optionalParamString = elt.SelectAttr("optional")
	rval.isConstParam = strings.HasPrefix(elt.InnerText(), "const")
	rval.pointerLevel = strings.Count(elt.InnerText(), "*")
	rval.lenSpec = elt.SelectAttr("len")
	rval.altLenSpec = elt.SelectAttr("altlen")

	rval.parentCommand = forCommand

	return &rval
}

func ReadCommandExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("command").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "comment" {
			return true
		} // Ignore comments

		entry := NewCommandFromJSON(key, exVal)
		tr[key.String()] = entry

		return true
	})
}

func NewCommandFromJSON(key, json gjson.Result) *commandType {
	rval := commandType{}
	rval.registryName = key.String()
	rval.publicName = json.Get("publicName").String()
	rval.staticCodeRef = json.Get("staticCodeRef").String()

	return &rval
}

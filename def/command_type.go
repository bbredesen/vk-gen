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
	strcase.ConfigureAcronym("Result", "r")
	strcase.ConfigureAcronym("PFN_vkVoidFunction", "fn")
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

func (t *commandType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return &includeSet{}
	}

	iset := &includeSet{}

	if t.aliasTypeName != "" {
		t.resolvedAliasType = tr[t.aliasTypeName]
		t.resolvedAliasType.Resolve(tr, vr)
	} else if t.returnTypeName != "" {
		t.resolvedReturnType = tr[t.returnTypeName]
		if t.resolvedReturnType == nil {
			log.WithField("registry name", t.registryName).
				WithField("return registry name", t.returnTypeName).
				Error("return type was not found while resolving command")
		} else {
			iset.MergeWith(t.resolvedReturnType.Resolve(tr, vr))

		}
	}

	if t.publicName == "" { // publicName may already be set by an entry from exceptions.json
		t.publicName = RenameIdentifier(t.registryName)
	}

	for _, p := range t.parameters {
		p.parentCommand = t

		iset.MergeWith(p.Resolve(tr, vr))
	}

	// t.identicalInternalExternal = t.determineIdentical()

	t.isResolved = true
	return iset
}

func (t *commandType) PrintGlobalDeclarations(w io.Writer, idx int) {
	if t.staticCodeRef != "" {
		// Ignored, static refs from exceptions.json aren't processed through
		// Cgo/lazyCommands
		return
	}

	if idx == 0 {
		fmt.Fprintf(w, "%s vkCommandKey = iota\n", t.keyName())
	} else {
		fmt.Fprintln(w, t.keyName())
	}
}

func (t *commandType) PrintFileInitContent(w io.Writer) {
	fmt.Fprintf(w, "lazyCommands[%s] = vkCommand{\"%s\", %d, %v, nil}\n",
		t.keyName(), t.RegistryName(), t.bindingParamCount, t.resolvedReturnType != nil)

}

func (t *commandType) PrintPublicDeclaration(w io.Writer) {
	if t.staticCodeRef != "" {
		fmt.Fprintf(w, "// %s is static code, not generated from vk.xml; aliased to %s\n", t.PublicName(), t.staticCodeRef)
		fmt.Fprintf(w, "var %s = %s\n\n", t.PublicName(), t.staticCodeRef)
		return
	}
	// funcReturnNames, funcReturnTypes := &strings.Builder{}, &strings.Builder{}

	preamble, epilogue := &strings.Builder{}, &strings.Builder{}

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
						fmt.Fprintf(preamble, "  sl_%s := make([]%s, %s)\n", p.publicName, paramTypeAsPointer.resolvedPointsAtType.InternalName(), p.lenMemberParam.publicName)
						fmt.Fprintf(preamble, "  for i, v := range %s {\n", p.publicName)
						fmt.Fprintf(preamble, "    sl_%s[i] = %s\n", p.publicName, paramTypeAsPointer.resolvedPointsAtType.TranslateToInternal("v"))
						fmt.Fprintf(preamble, "  }\n")
						fmt.Fprintf(preamble, "  %s := unsafe.Pointer(&sl_%s[0])\n", p.internalName, p.publicName)
						fmt.Fprintln(preamble)

						funcTrampolineParams = append(funcTrampolineParams, p)

					} else {
						// Parameter can be directly used (once we get a pointer
						// to the first element)
						fmt.Fprintf(preamble, "  // %s is an input slice of values that do not need translation used\n", p.publicName)
						fmt.Fprintf(preamble, "  %s := unsafe.Pointer(&%s[0])\n", p.internalName, p.publicName)
						fmt.Fprintln(preamble)
						funcTrampolineParams = append(funcTrampolineParams, p)
					}

				} else {
					// Parameter is a singular input
					if p.resolvedType.IsIdenticalPublicAndInternal() {
						fmt.Fprintf(preamble, "// Parameter is a singular input, pass direct - %s\n", p.publicName)
						fmt.Fprintf(preamble, "  %s := unsafe.Pointer(%s)\n", p.internalName, p.publicName)
						fmt.Fprintln(preamble)
						funcTrampolineParams = append(funcTrampolineParams, p)

					} else {
						fmt.Fprintf(preamble, "// Parameter is a singular input, requires translation - %s\n", p.publicName)
						fmt.Fprintf(preamble, "  %s := unsafe.Pointer(%s)\n", p.internalName, p.resolvedType.TranslateToInternal(p.publicName))
						fmt.Fprintln(preamble)
						funcTrampolineParams = append(funcTrampolineParams, p)

					}
				}
			} else {
				if p.lenMemberParam != nil { // lenSpec != "": spec might be a reference into another input, e.g. vkAllocateCommandBuffers, pAllocateInfo->commandBufferCount
					// Parameter is an output
					if p.lenMemberParam.resolvedType.Category() == CatPointer {
						// Parameter is a double-call array output
						fmt.Fprintf(preamble, "// %s is a double-call array output\n", p.publicName)
						// Allocate the length param and stub the slice
						fmt.Fprintf(preamble, "  var %s %s\n", p.lenMemberParam.publicName, p.lenMemberParam.resolvedType.(*pointerType).resolvedPointsAtType.PublicName())
						fmt.Fprintf(preamble, "  %s := unsafe.Pointer(&%s)\n", p.lenMemberParam.internalName, p.lenMemberParam.publicName)

						fmt.Fprintf(preamble, "  var %s unsafe.Pointer\n", p.internalName)

						// Get the length of the array that has to be allocated

						fmt.Fprintf(preamble, "// first trampoline happens here; also, check returned Result value\n")

						funcTrampolineParams = append(funcTrampolineParams, p.lenMemberParam)
						funcTrampolineParams = append(funcTrampolineParams, p)
						funcReturnParams = append(funcReturnParams, p)

						fmt.Fprintf(epilogue, "  %s = make(%s, %s)\n", p.publicName, p.resolvedType.PublicName(), p.lenMemberParam.publicName)
						fmt.Fprintf(epilogue, "  %s = unsafe.Pointer(&%s[0])\n", p.internalName, p.publicName)
						fmt.Fprintln(epilogue)
						t.printTrampolineCall(epilogue, funcTrampolineParams, trampolineReturns)
						fmt.Fprintln(epilogue)

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
							fmt.Fprintf(preamble, "// Parameter is a user-allocated array input that will be written to (?) - %s\n", p.publicName)
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
						} else {
							fmt.Fprintf(preamble, "// Parameter is binding-allocated array populated by Vulkan; length is provided by what? (%s) - %s\n", p.publicName, p.lenSpec)
							panic("Parameter is binding-allocated array populated by Vulkan, length not provided?? " + p.lenSpec)
						}
					} else {
						fmt.Fprintf(preamble, "// %s is a binding-allocated single return value and will be populated by Vulkan\n", p.publicName)
						fmt.Fprintf(preamble, "  %s := unsafe.Pointer(&%s)\n", p.internalName, p.publicName)
						fmt.Fprintln(preamble)

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
		fmt.Fprintf(sb, ", uintptr(%s)", param.internalName)
	}
	// Note that the leading ", " is not trimmed
	return sb.String()
}

func (t *commandType) printTrampolineCall(w io.Writer, trampParams []*commandParam, returnParam *commandParam) {
	trampParamsString := trampStringFromParams(trampParams)

	if returnParam != nil {
		fmt.Fprintf(w, "  %s = %s(execTrampoline(%s%s))\n", returnParam.publicName, returnParam.resolvedType.PublicName(), t.keyName(), trampParamsString)
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

	pointerLevel int
	lenSpec      string

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

func (p *commandParam) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if p.isResolved {
		return nil
	}

	iset := &includeSet{
		includeTypeNames: []string{p.typeName},
	}

	p.resolvedType = tr[p.typeName]
	iset.MergeWith(p.resolvedType.Resolve(tr, vr))

	// Build the pointer chain if applicable
	for i := 0; i < p.pointerLevel; i++ {
		ptr := pointerType{}
		ptr.resolvedPointsAtType = p.resolvedType
		p.resolvedType = &ptr
	}

	// check for length specification
	if p.lenSpec != "" {
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

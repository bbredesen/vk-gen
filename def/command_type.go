package def

import (
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/antchfx/xmlquery"
	log "github.com/sirupsen/logrus"
)

type commandType struct {
	genericType
	returnTypeName     string
	resolvedReturnType TypeDefiner

	parameters []*commandParam

	bindingParams []*commandParam
	returnParams  []*commandParam
	// identicalInternalExternal bool
	// isReturnedOnly            bool
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

func (t *commandType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return nil
	}

	iset := &includeSet{}

	if t.aliasRegistryName != "" {
		t.resolvedAliasType = tr[t.aliasRegistryName]
		t.resolvedAliasType.Resolve(tr, vr)
	} else if t.returnTypeName != "" {
		t.resolvedReturnType = tr[t.returnTypeName]
		if t.resolvedReturnType == nil {
			log.WithField("registry name", t.registryName).
				WithField("return registry name", t.returnTypeName).
				Error("return type was not found while resolving command")
		} else {
			iset.mergeWith(t.resolvedReturnType.Resolve(tr, vr))

		}
	}

	t.publicName = renameIdentifier(t.registryName)

	for _, p := range t.parameters {
		p.parentCommand = t
		iset.mergeWith(p.Resolve(tr, vr))
	}

	// t.identicalInternalExternal = t.determineIdentical()

	t.isResolved = true
	return iset
}

func (t *commandType) PrintGlobalDeclarations(w io.Writer, idx int) {
	if idx == 0 {
		fmt.Fprintf(w, "%s vkCommandKey = iota\n", t.keyName())
	} else {
		fmt.Fprintln(w, t.keyName())
	}
}

func (t *commandType) PrintFileInitContent(w io.Writer) {
	fmt.Fprintf(w, "lazyCommands[%s] = vkCommand{\"%s\", %d, %v, nil}\n",
		t.keyName(), t.RegistryName(), len(t.parameters), t.resolvedReturnType != nil)

}

func (t *commandType) PrintPublicDeclaration(w io.Writer) {
	funcParams := &strings.Builder{}
	trampParams := ""
	funcReturnNames, funcReturnTypes := &strings.Builder{}, &strings.Builder{}

	preamble, epilogue := &strings.Builder{}, &strings.Builder{}

	var isDoubleCall bool
	var doubleCalLenParam, doubleCallArrayParam *commandParam

	// for _, p := range t.bindingParams {
	// 	funcParams = funcParams + ", " + p.publicName + " " + p.resolvedType.PublicName()
	// }

	for _, p := range t.parameters {
		// Classes of parameters:
		// - Pure input, like an instance handle or create info
		// - Pure output of a single value in user-allocated memory; may be a primitive or a struct
		// - Pure output of an array, where memory needs to be pre-allocated
		// - Dual usage, output of array length then input of allocated array size

		// After classification, the params need to be in one or more of:
		// - Go function parameters
		// - Go return values
		// - Trampoline parameters, with or without translation

		var isInput, isOutput, isArray bool
		var isDoubleCallLength, isDoubleCallArray bool

		_, _, _, _ = isOutput, isDoubleCall, isDoubleCallLength, isDoubleCallArray

		if p.optionalParamString != "" && !p.isAlwaysOptional && p.pointerLevel > 0 && p.isLenMemberFor != nil {
			isDoubleCall = true
			isDoubleCallLength = true
			doubleCalLenParam = p
			doubleCallArrayParam = p.isLenMemberFor
		} else if p == doubleCallArrayParam {
			isOutput = true
			isDoubleCallArray = true
			fmt.Fprintf(funcReturnNames, ", %s", p.publicName)
			fmt.Fprintf(funcReturnTypes, ", []%s", p.resolvedType.PublicName())

		} else if p.isAlwaysOptional || p.isConstParam || p.pointerLevel == 0 {
			isInput = true
			// input parameters
			// funcParams = funcParams + ", " + p.publicName + " " + p.resolvedType.PublicName()
		} else {
			tname := p.resolvedType.PublicName()
			if p.lenSpec != "" {
				tname = "[]" + tname
				isArray = true
				var arrayLen string
				subLenSpecs := strings.Split(p.lenSpec, "->")
				if len(subLenSpecs) == 1 {
					arrayLen = subLenSpecs[0]
				} else {
					// Member field in one of the parameters
					structParam := t.findParam(subLenSpecs[0])
					arrayLen = fmt.Sprintf("%s.%s", structParam.publicName, strings.Title(subLenSpecs[1]))
				}

				fmt.Fprintf(preamble, "%s := make([]%s, %s)\n", p.publicName, p.resolvedType.InternalName(), arrayLen)
				fmt.Fprintf(funcReturnNames, ", %s", p.publicName)
			} else {
				fmt.Fprintf(funcReturnNames, ", %s", p.publicName)
			}

			fmt.Fprintf(funcReturnTypes, ", %s", tname)

			isOutput = true
		}

		if isDoubleCallLength {
			fmt.Fprintf(preamble, "var %s int\n", p.publicName)

			trampParams = trampParams + ", uintptr(unsafe.Pointer(&" + p.publicName + "))"

		} else if isInput {
			fmt.Fprintf(funcParams, ", %s %s", p.publicName, p.resolvedType.PublicName())
			// => ", varName Type"
			if p.resolvedType.IsIdenticalPublicAndInternal() {
				if p.resolvedType.Category() == CatStruct {
					trampParams = trampParams + ", uintptr(unsafe.Pointer(&" + p.publicName + "))"
				} else {
					trampParams = trampParams + ", uintptr(" + p.publicName + ")"
				}
			} else if isInput && p.resolvedType.Category() == CatStruct {
				fmt.Fprintf(preamble, "%s := %s.Vulkanize()\n", p.internalName, p.publicName)
				trampParams = trampParams + ", uintptr(unsafe.Pointer(" + p.internalName + "))"
			}
		}

		if isArray {
			fmt.Fprintf(preamble, "var addr_%s unsafe.Pointer\n", p.publicName)
			fmt.Fprintf(preamble, "if len(%s) > 0  {\n", p.publicName)
			fmt.Fprintf(preamble, "  addr_%s = unsafe.Pointer(&%s[0])\n", p.publicName, p.publicName)
			fmt.Fprintf(preamble, "}\n")

			trampParams = trampParams + ", uintptr(addr_" + p.publicName + ")"

			// fmt.Fprintf(epilogue, )

			// } else if isInput && p.resolvedType.IsIdenticalInternalExternal() {
			// 	trampParams = trampParams + ", uintptr(" + p.publicName + ")"
			// } else if isInput && p.resolvedType.Category() == Struct {
			// 	fmt.Fprintf(&preamble, "%s := %s.Vulkanize()\n", p.internalName, p.publicName)
			// 	trampParams = trampParams + ", uintptr(unsafe.Pointer(" + p.internalName + "))"
		} else if isDoubleCallArray {
			fmt.Fprintf(preamble, "var %s []%s\n", p.publicName, p.resolvedType.PublicName())
			trampParams = trampParams + ", uintptr(unsafe.Pointer(&" + p.publicName + "[0]))"

		} else if isOutput {
			fmt.Fprintf(preamble, "var %s %s\n", p.publicName, p.resolvedType.PublicName())
			trampParams = trampParams + ", uintptr(unsafe.Pointer(&" + p.publicName + "))"
		}
	}

	if t.resolvedReturnType != nil && t.resolvedReturnType.RegistryName() != "void" {
		fmt.Fprintf(preamble, "var r Result\n")
		fmt.Fprint(funcReturnNames, ", r")
		fmt.Fprintf(funcReturnTypes, ", %s", t.resolvedReturnType.PublicName())
	}

	funcReturnNamesStr := strings.TrimPrefix(funcReturnNames.String(), ", ")

	t.PrintDocLink(w)

	fmt.Fprintf(w, "func %s(%s) (%s) {\n", t.PublicName(), strings.TrimPrefix(funcParams.String(), ", "), strings.TrimPrefix(funcReturnTypes.String(), ", "))
	fmt.Fprintln(w, preamble.String())

	if isDoubleCall {
		initialParams := strings.Replace(trampParams, "&"+doubleCallArrayParam.publicName+"[0]", "nil", 1)
		t.printTrampolineCall(w, initialParams)
		if t.resolvedReturnType != nil && t.resolvedReturnType.RegistryName() != "void" {
			fmt.Fprintf(w, "if r != SUCCESS {\n")
			fmt.Fprintf(w, "  return %s\n", funcReturnNamesStr)
			fmt.Fprintf(w, "}\n")
			fmt.Fprintf(w, "%s = make([]%s, %s)\n\n", doubleCallArrayParam.publicName, doubleCallArrayParam.resolvedType.PublicName(), doubleCalLenParam.publicName)
		}
	}

	t.printTrampolineCall(w, trampParams)

	fmt.Fprintf(w, epilogue.String())
	if len(funcReturnNamesStr) > 0 {
		fmt.Fprintf(w, "  return %s\n", funcReturnNamesStr)
	}

	fmt.Fprintf(w, "}\n\n")
}

func (t *commandType) printTrampolineCall(w io.Writer, trampParams string) {
	if t.resolvedReturnType != nil && t.resolvedReturnType.RegistryName() != "void" {
		fmt.Fprintf(w, "  r = execTrampoline(%s%s)\n", t.keyName(), trampParams)
	} else {
		fmt.Fprintf(w, "  execTrampoline(%s%s)\n", t.keyName(), trampParams)
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
	isLenMemberFor *commandParam
}

func (p *commandParam) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if p.isResolved {
		return nil
	}

	iset := &includeSet{
		includeTypeNames: []string{p.typeName},
	}

	p.resolvedType = tr[p.typeName]
	p.resolvedType.Resolve(tr, vr)
	iset.mergeWith(p.resolvedType.Resolve(tr, vr))

	p.internalName = renameIdentifier(p.registryName)
	p.publicName = strings.TrimPrefix(p.internalName, "p")
	if p.publicName != "" {
		r, n := utf8.DecodeRuneInString(p.publicName)
		p.publicName = string(unicode.ToLower(r)) + p.publicName[n:]
	}

	p.isAlwaysOptional = p.optionalParamString == "true"

	if p.lenSpec != "" {
		for _, otherP := range p.parentCommand.parameters {
			if otherP.registryName == p.lenSpec {
				otherP.isLenMemberFor = p
			}
		}
	}

	p.isResolved = true

	if p.isAlwaysOptional || p.isConstParam || p.pointerLevel == 0 {
		p.parentCommand.bindingParams = append(p.parentCommand.bindingParams, p)
	} else {
		p.parentCommand.returnParams = append(p.parentCommand.returnParams, p)
	}

	return iset
}

func ReadCommandTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry) {
	for _, commandNode := range xmlquery.Find(doc, "//commands/command") {
		val := NewCommandFromXML(commandNode)
		tr[val.RegistryName()] = val
	}
}

func NewCommandFromXML(elt *xmlquery.Node) *commandType {
	rval := commandType{}
	name := elt.SelectAttr("name")
	if name != "" {
		rval.registryName = name
		rval.aliasRegistryName = elt.SelectAttr("alias")
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

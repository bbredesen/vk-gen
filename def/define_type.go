package def

import (
	"fmt"
	"io"
	"regexp"
	"unsafe"

	"github.com/antchfx/xmlquery"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type defineType struct {
	internalType

	functionName, paramString, valueString string
}

func (t *defineType) Category() TypeCategory { return CatDefine }

func (t *defineType) Resolve(tr TypeRegistry, vr ValueRegistry) *includeSet {
	if t.isResolved {
		return &includeSet{}
	}

	rval := t.internalType.Resolve(tr, vr)

	t.functionName = RenameIdentifier(t.functionName)
	t.valueString = RenameIdentifier(t.valueString)
	t.paramString = rxParamSearch.ReplaceAllStringFunc(t.paramString, RenameIdentifier)

	t.isResolved = true
	return rval
}

var rxParamSearch = regexp.MustCompile(`VK\w+`)

func (t *defineType) PrintPublicDeclaration(w io.Writer) {

	t.PrintDocLink(w)

	if t.paramString != "" {
		fmt.Fprintf(w, "var %s = %s%s\n", t.PublicName(), t.functionName, t.paramString)
	} else if t.functionName != "" {
		fmt.Fprintf(w, "var %s = %s\n", t.PublicName(), t.functionName)
	} else if t.valueString != "" {
		// uint32 here is a hack. There is only one define as of 1.2.190, VK_HEADER_VERSION (vk.xml
		// version used for development) that is a value, not a macro. Value
		// gets passed to a function expecting uint32.
		fmt.Fprintf(w, "var %s = uint32(%s)\n", t.PublicName(), t.valueString)
	} else {
		fmt.Fprintf(w, "4 other define for %s\n", t.PublicName())
	}
}

func ReadDefineTypesFromXML(doc *xmlquery.Node, tr TypeRegistry, _ ValueRegistry) {
	for _, node := range xmlquery.Find(doc, "//types/type[@category='define']") {
		newType := NewDefineTypeFromXML(node)
		if tr[newType.RegistryName()] != nil {
			logrus.WithField("registry name", newType.RegistryName()).
				Warn("Attempted overwrite of define type from XML\n")
		} else {
			tr[newType.RegistryName()] = newType
		}
	}
}

var s = unsafe.Sizeof(uintptr(0))

// NewDefineTypeFromXML has a few procesing paths. First, the name is typically
// a child node, but is an XML attribute for a few entries. Next, some defines
// such as VK_API_VERSION_1_0 are actually macro calls, which is named in a
// child node for type. Defines may also have a requires attr, which also
// indicates the name of the macro call, except when it doesn't (e.g.,
// VK_HEADER_VERSION_COMPLETE). Then, parameters of the macro may be
// present in plain text between parenthesis. Or the define might be a singular
// value, in the case of VK_HEADER_VERSION.
func NewDefineTypeFromXML(node *xmlquery.Node) *defineType {
	rval := defineType{}

	if nnode := xmlquery.FindOne(node, "/name"); nnode != nil {
		rval.registryName = nnode.InnerText()
	} else {
		rval.registryName = node.SelectAttr("name")
	}

	searchNodeIdent := "name"

	if tnode := xmlquery.FindOne(node, "/type"); tnode != nil {
		rval.functionName = tnode.InnerText()
		searchNodeIdent = "type"
	}

	if requiresType := node.SelectAttr("requires"); requiresType != "" {
		rval.requiresTypeName = requiresType
		//  NEEDS TO BE HANDLED IN RESOLVE()
	}

	q := fmt.Sprintf("/%s/following-sibling::text()", searchNodeIdent)

	if textData := xmlquery.FindOne(node, q); textData != nil {
		matches := defineParamsRx.FindStringSubmatch(textData.Data)
		if len(matches) > 0 {
			if rval.functionName != "" {
				rval.paramString = matches[1] //  Match zero is the full string
			} else {
				rval.valueString = matches[1]
			}
		}
	}

	return &rval
}

var defineParamsRx = regexp.MustCompile(`((?:\(.*\))|(?:\d+))(?:\/\/){0,1}.*$`)

func ReadDefineExceptionsFromJSON(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry) {
	exceptions.Get("define").ForEach(func(key, exVal gjson.Result) bool {
		if key.String() == "!comment" {
			return true
		} // Ignore comments

		if exVal.Type == gjson.String {
			if exVal.String() == "!ignore" {
				delete(tr, key.String())
				return true
			} else {
				logrus.WithField("key", key.String()).
					WithField("value", exVal.String()).
					Fatal("Fatal error when reading define exceptions: value for this key must be an object or the string \"!ignore\"")
			}
		}

		entry := NewDefineTypeFromJSON(key, exVal)
		if entry == nil {
			// nil means that the !ignore key was found in the object.
			delete(tr, key.String())
			return true
		}

		tr[entry.RegistryName()] = entry

		// exVal.Get("constants").ForEach(func(ck, cv gjson.Result) bool {
		// 	newVal := NewConstantValue(ck.String(), cv.String(), key.String())

		// 	vr[newVal.RegistryName()] = newVal
		// 	return true
		// })

		return true
	})

}

func NewDefineTypeFromJSON(key, json gjson.Result) TypeDefiner {
	if json.Get("!ignore").Bool() {
		return nil
	}

	rval := defineType{}
	rval.registryName = key.String()

	rval.functionName = json.Get("functionName").String()
	rval.valueString = json.Get("constantValue").String()

	rval.publicName = json.Get("publicName").String()
	rval.underlyingTypeName = json.Get("underlyingType").String()

	rval.comment = json.Get("comment").String()

	return &rval
}

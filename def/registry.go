package def

//go:generate stringer -type TypeCategory registry.go

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/tidwall/gjson"
)

type TypeCategory int

const (
	CatNone TypeCategory = iota

	CatGeneric
	CatPrimitive
	CatInclude
	CatBasetype
	CatStatic

	CatPointer

	CatHandle
	CatEnum
	CatBitmask
	CatStruct

	CatCommand

	CatMaximum
)

type TypeRegistry map[string]TypeDefiner
type ValueRegistry map[string]ValueDefiner

func (t TypeRegistry) FilterByCategory() map[TypeCategory]TypeRegistry {
	rval := make(map[TypeCategory]TypeRegistry)

	for k, v := range t {
		subReg, found := rval[v.Category()]
		if !found {
			subReg = make(TypeRegistry)
			rval[v.Category()] = subReg
		}
		subReg[k] = v
	}

	return rval
}

type fnReadFromXML func(doc *xmlquery.Node, tr TypeRegistry, vr ValueRegistry)
type fnReadFromJSON func(exceptions gjson.Result, tr TypeRegistry, vr ValueRegistry)

func (c TypeCategory) ReadFns() (fnReadFromXML, fnReadFromJSON) {
	switch c {

	// case CatGeneric:
	case CatPrimitive:
		return ReadAPIConstantsFromXML, nil
	case CatInclude:
		return ReadIncludeTypesFromXML, ReadIncludeExceptionsFromJSON
	case CatBasetype:
		return ReadBaseTypesFromXML, ReadBaseTypeExceptionsFromJSON
	// case CatStatic:
	// case CatPointer:

	case CatHandle:
		return ReadHandleTypesFromXML, ReadHandleExceptionsFromJSON

	case CatEnum:
		return ReadEnumTypesFromXML, nil
	case CatBitmask:
		return ReadBitmaskTypesFromXML, nil
	case CatStruct:
		return ReadStructTypesFromXML, nil

	case CatCommand:
		return ReadCommandTypesFromXML, nil

	default:
		return nil, nil
	}
}

type ByName []TypeDefiner

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].PublicName() < a[j].PublicName() }

type byValue []ValueDefiner

func (a byValue) Len() int      { return len(a) }
func (a byValue) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byValue) Less(i, j int) bool {
	iNum, err1 := strconv.Atoi(a[i].ValueString())
	jNum, err2 := strconv.Atoi(a[j].ValueString())
	if err1 == nil && err2 == nil {
		return iNum < jNum
	}
	return a[i].ValueString() < a[j].ValueString()
}

func WriteStringerCommands(w io.Writer, defs []TypeDefiner, cat TypeCategory) {
	types := ""
	i := 0
	fileCount := 0

	// catString := strings.ToLower(cat.String())
	catString := strings.ToLower(strings.TrimPrefix(cat.String(), "Cat"))

	for j, v := range defs {

		if v.Category() == cat && len(v.AllValues()) > 0 {
			types += v.PublicName() + ","
			i++
		} else {
			continue
		}

		if i == 63 || j == len(defs)-1 { // Limit to max 64 types per call to stringer
			outFile := fmt.Sprintf("%s_string_%d.go", catString, fileCount)
			types = types[:len(types)-1]
			fmt.Fprintf(w, "//go:generate stringer -output=%s -type=%s\n", outFile, types)

			types = ""
			fileCount++
			i = 0
		}
	}
}

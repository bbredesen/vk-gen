package def

import (
	"strings"
	"testing"
)

func TestAbiClassOf(t *testing.T) {
	ext := func(name string) *externalType {
		e := &externalType{}
		e.registryName = name
		return e
	}
	base := func(under TypeDefiner) *baseType {
		b := &baseType{}
		b.resolvedUnderlyingType = under
		return b
	}

	tests := []struct {
		name string
		td   TypeDefiner
		want byte
		err  bool
	}{
		{"float", ext("float"), classFloat, false},
		{"double", ext("double"), classDouble, false},
		{"uint32_t", ext("uint32_t"), classInt, false},
		{"basetype over float", base(ext("float")), classFloat, false},
		{"basetype of basetype over uint64_t", base(base(ext("uint64_t"))), classInt, false},
		{"pointer to float", &pointerType{resolvedPointsAtType: ext("float")}, classInt, false},
		{"handle", &handleType{}, classInt, false},
		{"enum", &enumType{}, classInt, false},
		{"bitmask", &bitmaskType{}, classInt, false},
		{"struct by value", &structType{}, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := abiClassOf(tc.td)
			if (err != nil) != tc.err {
				t.Fatalf("error = %v, want error: %v", err, tc.err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShapeOfParams(t *testing.T) {
	f := &externalType{}
	f.registryName = "float"
	i := &externalType{}
	i.registryName = "int"

	params := []*commandParam{
		{registryName: "a", resolvedType: &handleType{}},
		{registryName: "b", resolvedType: f},
		{registryName: "c", resolvedType: &pointerType{resolvedPointsAtType: f}},
		{registryName: "d", resolvedType: i},
	}
	got, err := shapeOfParams(params)
	if err != nil || got != "ifii" {
		t.Errorf("got %q, %v; want ifii", got, err)
	}
}

// Every Go wrapper must carry //go:uintptrescapes. Generated commands pass pointers as uintptr(unsafe.Pointer(x)); without
// the directive the pointer target can be freed or (if on a goroutine stack) moved while C is using it.
func TestWrappersHaveUintptrEscapes(t *testing.T) {
	shapes := []string{"", "i", "if", "iff", "iiiiiiiiiiiiiii", "id"}

	src, err := renderTrampolineSources(shapes)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(src.goWrapper), "\n")
	found := 0
	for i, line := range lines {
		if !strings.HasPrefix(line, "func vkCall_") {
			continue
		}
		found++
		if i == 0 || lines[i-1] != "//go:uintptrescapes" {
			t.Errorf("wrapper lacks //go:uintptrescapes on the line above it: %s", line)
		}
	}
	if found != len(shapes) {
		t.Errorf("found %d wrappers, want %d", found, len(shapes))
	}
}

func TestTrampolineTypes(t *testing.T) {
	src, err := renderTrampolineSources([]string{"ifd"})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"uintptr_t vkTramp_ifd(void *fn, uintptr_t p0, float p1, double p2)",
		"((uintptr_t (*)(uintptr_t, float, double))fn)(p0, p1, p2)",
	} {
		if !strings.Contains(src.c, want) {
			t.Errorf("trampolines.c is missing %q", want)
		}
	}
	if want := "func vkCall_ifd(cmd *vkCommand, p0 uintptr, p1 float32, p2 float64) uintptr"; !strings.Contains(string(src.goWrapper), want) {
		t.Errorf("shapes.go is missing %q", want)
	}
}

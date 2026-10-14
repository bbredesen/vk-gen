package def

import "testing"

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

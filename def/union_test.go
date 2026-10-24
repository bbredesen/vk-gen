package def

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/antchfx/xmlquery"
	"github.com/tidwall/gjson"
)

// registryFile is a pinned registry from the matrix tests. It is committed, so these tests do not depend on a vk.xml in
// the working directory (which .gitignore excludes, and which CI does not have).
const registryFile = "../matrix/testdata/registry/vk-1.4.366.xml"

// loadRegistries reads the pinned registry and exceptions.json the same way main does.
func loadRegistries(t *testing.T) (TypeRegistry, ValueRegistry) {
	t.Helper()

	f, err := os.Open(registryFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	doc, err := xmlquery.Parse(f)
	if err != nil {
		t.Fatal(err)
	}

	exBytes, err := os.ReadFile("../exceptions.json")
	if err != nil {
		t.Fatal(err)
	}
	ex := gjson.ParseBytes(exBytes)

	tr, vr := make(TypeRegistry), make(ValueRegistry)
	for tc := CatNone; tc < CatMaximum; tc++ {
		xml, json := tc.ReadFns()
		if xml != nil {
			xml(doc, tr, vr, "vulkan")
		}
		if json != nil {
			json(ex, tr, vr)
		}
	}
	return tr, vr
}

func printed(td TypeDefiner, fn func(TypeDefiner, *bytes.Buffer)) string {
	var b bytes.Buffer
	fn(td, &b)
	return b.String()
}

func TestReturnedUnionSelectors(t *testing.T) {
	tr, vr := loadRegistries(t)

	cases := []struct {
		parent, union string
		wantUnion     []string
		wantParent    string
	}{
		{
			parent: "VkPerformanceValueINTEL",
			union:  "VkPerformanceValueDataINTEL",
			wantUnion: []string{
				"selected by PerformanceValueINTEL.Typ",
				"PERFORMANCE_VALUE_TYPE_UINT32_INTEL -> Value32",
				"func (u *PerformanceValueDataINTEL) IsValueString() bool",
				"Goify(selector PerformanceValueTypeINTEL) *PerformanceValueDataINTEL",
				"case PERFORMANCE_VALUE_TYPE_STRING_INTEL:",
				"rval.ValueString = cStringToString(",
			},
			wantParent: "Data : *(s.data.Goify(PerformanceValueTypeINTEL(s.typ)))",
		},
		{
			parent: "VkPipelineExecutableStatisticKHR",
			union:  "VkPipelineExecutableStatisticValueKHR",
			wantUnion: []string{
				"selected by PipelineExecutableStatisticKHR.Format",
				"PIPELINE_EXECUTABLE_STATISTIC_FORMAT_FLOAT64_KHR -> F64",
				"func (u *PipelineExecutableStatisticValueKHR) IsF64() bool",
				"case PIPELINE_EXECUTABLE_STATISTIC_FORMAT_UINT64_KHR:",
				"rval.asU64 = true",
			},
			wantParent: "Value : *(s.value.Goify(PipelineExecutableStatisticFormatKHR(s.format)))",
		},
	}

	for _, c := range cases {
		t.Run(c.union, func(t *testing.T) {
			parent, union := tr[c.parent], tr[c.union]
			parent.Resolve(tr, vr)

			pub := printed(union, func(td TypeDefiner, b *bytes.Buffer) { td.PrintPublicDeclaration(b) })
			internal := printed(union, func(td TypeDefiner, b *bytes.Buffer) { td.PrintInternalDeclaration(b) })
			parentInternal := printed(parent, func(td TypeDefiner, b *bytes.Buffer) { td.PrintInternalDeclaration(b) })

			all := pub + internal
			for _, want := range c.wantUnion {
				if !strings.Contains(all, want) {
					t.Errorf("union output missing %q\n%s", want, all)
				}
			}
			if !strings.Contains(parentInternal, c.wantParent) {
				t.Errorf("parent Goify missing %q\n%s", c.wantParent, parentInternal)
			}
		})
	}
}

// Input-only unions with a selector, whose members are structs, must not get a Goify.
func TestStructMemberUnionNotGoified(t *testing.T) {
	tr, vr := loadRegistries(t)

	tr["VkAccelerationStructureGeometryKHR"].Resolve(tr, vr)
	u := tr["VkAccelerationStructureGeometryDataKHR"]

	out := printed(u, func(td TypeDefiner, b *bytes.Buffer) { td.PrintInternalDeclaration(b) })
	if strings.Contains(out, ") Goify(") {
		t.Errorf("unexpected Goify for union with struct members:\n%s", out)
	}
}

// VkPerformanceCounterResultKHR has no selector in vk.xml; exceptions.json supplies one.
func TestExceptionSelectedUnion(t *testing.T) {
	tr, vr := loadRegistries(t)

	u := tr["VkPerformanceCounterResultKHR"]
	u.Resolve(tr, vr)

	all := printed(u, func(td TypeDefiner, b *bytes.Buffer) { td.PrintPublicDeclaration(b) }) +
		printed(u, func(td TypeDefiner, b *bytes.Buffer) { td.PrintInternalDeclaration(b) })

	for _, want := range []string{
		"supply (see PerformanceCounterResultKHRFromBytes)",
		"case PERFORMANCE_COUNTER_STORAGE_UINT64_KHR:",
		"func PerformanceCounterResultKHRFromBytes(b []byte, selector PerformanceCounterStorageKHR) *PerformanceCounterResultKHR",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("output missing %q\n%s", want, all)
		}
	}
}

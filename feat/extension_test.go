package feat

import (
	"strings"
	"testing"

	"github.com/antchfx/xmlquery"
	"github.com/bbredesen/vk-gen/def"
)

// Regression test: extension enums extending a bitmask must keep their bitpos / explicit value instead of being
// assigned the extension-number offset formula (1000000000 + (extNum-1)*1000 + offset).
const bitmaskExtensionXML = `<registry>
  <types>
    <type category="bitmask" requires="VkFooFlagBits">typedef <type>VkFlags</type> <name>VkFooFlags</name>;</type>
    <type category="enum" name="VkFooFlagBits"/>
    <type category="enum" name="VkBarType"/>
  </types>
  <enums name="VkFooFlagBits" type="bitmask">
    <enum bitpos="0" name="VK_FOO_A_BIT"/>
  </enums>
  <enums name="VkBarType" type="enum">
    <enum value="0" name="VK_BAR_TYPE_A"/>
  </enums>
  <extensions>
    <extension name="VK_KHR_test" number="81" supported="vulkan">
      <require>
        <enum bitpos="3" extends="VkFooFlagBits" name="VK_FOO_PUSH_BIT_KHR" comment="pushed"/>
        <enum value="0" extends="VkFooFlagBits" name="VK_FOO_NONE_KHR"/>
        <enum value="1" extends="VkBarType" name="VK_BAR_TYPE_EXPLICIT_KHR"/>
        <enum offset="2" extends="VkBarType" name="VK_BAR_TYPE_OFFSET_KHR"/>
      </require>
    </extension>
  </extensions>
</registry>`

func TestExtensionBitmaskValues(t *testing.T) {
	doc, err := xmlquery.Parse(strings.NewReader(bitmaskExtensionXML))
	if err != nil {
		t.Fatal(err)
	}

	tr := make(def.TypeRegistry)
	vr := make(def.ValueRegistry)
	for _, cat := range []def.TypeCategory{def.CatEnum, def.CatBitmask} {
		read, _ := cat.ReadFns()
		read(doc, tr, vr, "vulkan")
	}
	ReadExtensionFromXML(xmlquery.FindOne(doc, "//extension"), tr, vr)

	tests := []struct {
		name, want string
	}{
		{"VK_FOO_A_BIT", "1 << 0"},
		{"VK_FOO_PUSH_BIT_KHR", "1 << 3"},
		{"VK_FOO_NONE_KHR", "0"},
		{"VK_BAR_TYPE_EXPLICIT_KHR", "1"},
		{"VK_BAR_TYPE_OFFSET_KHR", "1000080002"},
	}
	for _, tc := range tests {
		v, ok := vr[tc.name]
		if !ok {
			t.Errorf("%s: not in value registry", tc.name)
			continue
		}
		if got := v.ValueString(); got != tc.want {
			t.Errorf("%s: got value %q, want %q", tc.name, got, tc.want)
		}
	}
}

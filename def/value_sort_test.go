package def

import (
	"math/rand"
	"sort"
	"testing"
)

func testLiteral(name, value string) ValueDefiner {
	v := &enumValue{}
	v.registryName = name
	v.valueString = value
	return v
}

func testAlias(name string, target ValueDefiner) ValueDefiner {
	v := &enumValue{}
	v.registryName = name
	v.aliasValueName = target.RegistryName()
	v.resolvedAliasValue = target
	return v
}

func valueNames(vs []ValueDefiner) []string {
	rval := make([]string, len(vs))
	for i, v := range vs {
		rval[i] = v.RegistryName()
	}
	return rval
}

func TestByValueOrder(t *testing.T) {
	b2 := testLiteral("B2", "1 << 2")
	b10 := testLiteral("B10", "1 << 10")
	b63 := testLiteral("B63", "1 << 63")
	hexAll := testLiteral("ALL", "0x1F")
	none := testLiteral("NONE", "0")
	neg := testLiteral("NEG", "-1")
	ext := testLiteral("EXT", "1000080000")
	dec9 := testLiteral("DEC9", "9")
	other := testLiteral("OTHER", "(~0U)")

	// Aliases trail, ordered by their target's value, and alias-of-alias resolves through to the final target
	aliasB10 := testAlias("ALIAS_B10", b10)
	aliasB2 := testAlias("ALIAS_B2", b2)
	aliasAliasB2 := testAlias("ALIAS_ALIAS_B2", aliasB2)

	// 1 << 63 is larger than 1000080000, so B63 must follow EXT
	want := []string{"NEG", "NONE", "B2", "DEC9", "ALL", "B10", "EXT", "B63", "OTHER", "ALIAS_ALIAS_B2", "ALIAS_B2", "ALIAS_B10"}

	all := []ValueDefiner{b2, b10, b63, hexAll, none, neg, ext, dec9, other, aliasB10, aliasB2, aliasAliasB2}

	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		shuffled := append([]ValueDefiner(nil), all...)
		rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		sort.Sort(ByValue(shuffled))

		got := valueNames(shuffled)
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("shuffle %d: got order %v, want %v", i, got, want)
			}
		}
	}
}

func TestByValueTieBreaksByName(t *testing.T) {
	a := testLiteral("VK_A", "1 << 3")
	b := testLiteral("VK_B", "8")
	vs := ByValue{b, a}
	sort.Sort(vs)
	if got := valueNames(vs); got[0] != "VK_A" || got[1] != "VK_B" {
		t.Errorf("equal values should order by name, got %v", got)
	}
}

func TestParseValueNumber(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "-7": -7, "0x10": 16, "1 << 5": 32, "1000000000": 1000000000} {
		if got := parseValueNumber(in); got == nil || got.Int64() != want {
			t.Errorf("parseValueNumber(%q) = %v, want %d", in, got, want)
		}
	}
	for _, in := range []string{"", "(~0U)", "SOME_NAME", "1 << x", "1 << 999"} {
		if got := parseValueNumber(in); got != nil {
			t.Errorf("parseValueNumber(%q) = %v, want nil", in, got)
		}
	}
}

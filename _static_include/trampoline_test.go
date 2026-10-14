package vk

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// This test builds a small fake "Vulkan" library whose functions have the real C signatures of the commands that take
// floats by value, calls them through the generated bindings, and checks that the callee received the right values.
// The library records its arguments; they are read back through the same trampoline machinery using integer-only
// functions (float bits travel as integers).
//
// It needs a C compiler on PATH and is skipped otherwise.

const fakeVulkanC = `
#include <stdint.h>
#include <string.h>

static float    g_f[4];
static uint64_t g_i[4];

void vkCmdSetLineWidth(uintptr_t cb, float w) { g_i[0] = cb; g_f[0] = w; }
void vkCmdSetDepthBias(uintptr_t cb, float a, float b, float c) { g_i[0] = cb; g_f[0] = a; g_f[1] = b; g_f[2] = c; }
void vkCmdSetDepthBounds(uintptr_t cb, float a, float b) { g_i[0] = cb; g_f[0] = a; g_f[1] = b; }
void vkSetDeviceMemoryPriorityEXT(uintptr_t dev, uint64_t mem, float p) { g_i[0] = dev; g_i[1] = mem; g_f[0] = p; }

uintptr_t testGetFloatBits(uintptr_t i) { uint32_t u; memcpy(&u, &g_f[i], sizeof u); return u; }
uintptr_t testGetInt(uintptr_t i) { return (uintptr_t)g_i[i]; }
void testReset(void) { memset(g_f, 0, sizeof g_f); memset(g_i, 0, sizeof g_i); }
`

var (
	cmdGetFloatBits = &vkCommand{"testGetFloatBits", 1, true, nil}
	cmdGetInt       = &vkCommand{"testGetInt", 1, true, nil}
	// testReset takes no arguments, but no generated command has an empty shape; the extra argument is ignored by the callee
	cmdReset = &vkCommand{"testReset", 1, false, nil}
)

func gotFloat(i uintptr) float32 {
	return math.Float32frombits(uint32(vkCall_i(cmdGetFloatBits, i)))
}

func gotInt(i uintptr) uintptr { return vkCall_i(cmdGetInt, i) }

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		os.Exit(m.Run())
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		println("no C compiler found, skipping trampoline tests")
		os.Exit(0)
	}

	dir, err := os.MkdirTemp("", "vkfake")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "fake.c")
	if err := os.WriteFile(src, []byte(fakeVulkanC), 0666); err != nil {
		panic(err)
	}

	lib := filepath.Join(dir, "libfakevulkan.so")
	args := []string{"-shared", "-fPIC", "-o", lib, src}
	if runtime.GOOS == "darwin" {
		args = []string{"-dynamiclib", "-o", lib, src}
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		println("could not build fake Vulkan library:", string(out))
		os.Exit(1)
	}

	OverrideDefaultVulkanLibrary(lib)
	os.Exit(m.Run())
}

func TestFloatArgs(t *testing.T) {
	tests := []struct {
		name   string
		call   func()
		ints   []uintptr
		floats []float32
	}{
		{"CmdSetLineWidth", func() { CmdSetLineWidth(CommandBuffer(0x1234), 2.5) },
			[]uintptr{0x1234}, []float32{2.5}},
		{"CmdSetDepthBias", func() { CmdSetDepthBias(CommandBuffer(0x1234), 1.5, -2.25, 3.75) },
			[]uintptr{0x1234}, []float32{1.5, -2.25, 3.75}},
		{"CmdSetDepthBounds", func() { CmdSetDepthBounds(CommandBuffer(0x1234), 0.25, 0.75) },
			[]uintptr{0x1234}, []float32{0.25, 0.75}},
		{"SetDeviceMemoryPriorityEXT", func() { SetDeviceMemoryPriorityEXT(Device(0x1234), DeviceMemory(0x5678), 0.5) },
			[]uintptr{0x1234, 0x5678}, []float32{0.5}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vkCall_i(cmdReset, 0)
			tc.call()

			for i, want := range tc.ints {
				if got := gotInt(uintptr(i)); got != want {
					t.Errorf("integer arg %d: got %#x, want %#x", i, got, want)
				}
			}
			for i, want := range tc.floats {
				if got := gotFloat(uintptr(i)); got != want {
					t.Errorf("float arg %d: got %v, want %v", i, got, want)
				}
			}
		})
	}
}

// TestIntegerOnlyShapeLosesFloats is a negative control documenting why shapes exist: calling a float-taking function
// through an all-integer trampoline puts the float bits in an integer register, where the callee does not look for them.
func TestIntegerOnlyShapeLosesFloats(t *testing.T) {
	wrong := &vkCommand{"vkCmdSetLineWidth", 2, false, nil}

	vkCall_i(cmdReset, 0)
	vkCall_ii(wrong, 0x1234, uintptr(math.Float32bits(2.5)))

	if got := gotFloat(0); got == 2.5 {
		t.Fatalf("float arrived intact through an integer-only trampoline; shapes may no longer be necessary on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

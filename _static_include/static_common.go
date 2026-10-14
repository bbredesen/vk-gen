package vk

import (
	"bytes"
	"runtime"
	"unsafe"
)

// #include <stdlib.h>
// #include "dlload.h"
import "C"

// Vulkanizer allows conversion from go-vk style structs to Vulkan-native structs. This
// includes setting the structure type flag, converting slices to pointers, etc.
type Vulkanizer interface {
	Vulkanize() unsafe.Pointer
}

// Goifier converts Vulkan-native structs back into go-vk style structs
type Goifier interface {
	Goify() Vulkanizer
}

// max is an internal utility function, used in processing struct member slice/array lengths
func max(nums ...int) int {
	rval := 0
	for _, v := range nums {
		if rval < v {
			rval = v
		}
	}
	return rval
}

// Error implements the error interface
// TODO: A way for commands to indicate if the Result code is an error for that command, or an unexpected return value?
func (r Result) Error() string {
	return r.String()
}

type vkCommand struct {
	protoName string
	argCount  int
	hasReturn bool
	fnHandle  unsafe.Pointer
}

var dlHandle unsafe.Pointer

var overrideLibName string

// OverrideDefaultVulkanLibrary allows you to set a specific Vulkan library name to be used in your program. For
// example, if you want to enable the validation layers, those layers are only available in the Vulkan SDK libary. go-vk
// passes the name to the host operating system's library opening/search method, so you must provide a relative or
// absolute path if your Vulkan library is not in the default search path for the platform.
func OverrideDefaultVulkanLibrary(nameOrPath string) {
	overrideLibName = nameOrPath
}

// resolve loads the Vulkan library on first use, looks up the command's symbol on its first call, and returns the
// function pointer. The typed trampoline wrappers in shapes.go call this before each invocation.
func (cmd *vkCommand) resolve() unsafe.Pointer {
	if dlHandle == nil {
		var libName string
		switch runtime.GOOS {
		case "windows":
			libName = "vulkan-1.dll"
		case "darwin":
			libName = "libMoltenVK.dylib"
		case "linux":
			// TODO: Running on Linux is tested only to the point of creating and
			// destroying a Vulkan instance.
			libName = "libvulkan.so"
		default:
			panic("Unsupported GOOS at OpenLibrary: " + runtime.GOOS)
		}

		if overrideLibName != "" {
			libName = overrideLibName
		}

		cstr := C.CString(libName)
		dlHandle = C.OpenLibrary(cstr)
		C.free(unsafe.Pointer(cstr))
	}

	// cmd := lazyCommands[commandKey]
	if cmd.fnHandle == nil {
		cmd.fnHandle = C.SymbolFromName(dlHandle, unsafe.Pointer(sys_stringToBytePointer(cmd.protoName)))
		// lazyCommands[commandKey] = cmd
	}

	return cmd.fnHandle
}

func stringToNullTermBytes(s string) *byte {
	b := []byte(s)
	b = append(b, 0)
	return &b[0]
}

func nullTermBytesToString(b []byte) string {
	n := bytes.IndexByte(b, 0)
	return string(b[:n])
}

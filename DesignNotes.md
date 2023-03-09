# vk-gen Design

vk.xml schema documentation: [https://registry.khronos.org/vulkan/specs/1.3/registry.html]

This file documents some of the thought process and tasks during development of vk-gen. This file remains for historical
reference, but the codebase has grown beyond what is detailed here and you should not assume that this file describes
precisely how the tool works. 

## vk.xml and exceptions.json
There are several significant barriers to generating the Go-binding for Vulkan:

1) vk.xml includes a lot of C-preprocessor macros, included types from other
   headers, and C syntax for some bitmasks; all features that cannot be easily
   translated to Go in an automatic fashion.
1) Pointer types in C map to several different representations in Go. For
   example, a void pointer will generally be an unsafe.Pointer in Go, a char* is
   a string (but there are also fixed length char arrays in the spec), other
   pointers types are slices of structs, and a char** is a slice of strings.
1) There are a small number of Vulkan commands and structs whose semantics are
   inconsistent with the rest of the library.
1) There is at least one function (an Nvidia extension) that uses C bitfield parameters (a :8 and :24 to combine two
   params into one 32-bit word), which doesn't have an analogue in Go.

Together, this means that we need a structured set of exceptions or overrides
for the tool to read, implemented as exceptions.json. This reduces
maintainability and potentially requires new exceptions be added on API updates.

## Vulkan API Quirks

This section is **not** a criticism of the Vulkan API design. The intention is
simply to document quirks and inconsistent treatment of certain fields in vk.xml
that have to be addressed by this tool.

## Reading TypeDefiners

The discrete types in the Vulkan specification are read into various structs
implementing the TypeDefiner interface.

### define

The API specifies a number of "define" types, which are C preprocessor
directives. Because of the complexity involved in parsing and translating that
code, many of the define types are redefined or annotated in exceptions.json.

A `define` map has entries that can resolve either to a command name, to a
function call, or to a static value. Each entry in the map is named for the
Vulkan macro name, e.g., `VK_MAKE_API_VERSION`.

A map entry is an object with the following fields:
* `!ignore` - if this field is set to `true` then this mapping and any matching
  entry from the XML spec will be ignored. Alternatively, the string `"!ignore"`
  can be used in place of this object.
* `functionName` - The name of a valid Go function in the library; will cause an
  error if used with `constantValue`
* `constantValue` - A fixed value to output in Go code; will cause an error if
  used with `functionName`
* `publicName` - A string to override the mapping and removal of vk from
  identifiers. The publicName of a type is what appears on the developer side of
  the generated code.
* `underlyingType` - The registry type name to use for this entry.
* `comment` - Arbitrary user data that will be output as a comment above this
  entity in the generated code.
* `!comment` - Arbitrary user data that will be ignored (i.e., intended as a
  comment on the JSON file, not as an output comment)

### include

In the specification a type with category="include" is specifically intended to
contain legal C code with a `#include` preprocessor directive.
[10.2.3](https://registry.khronos.org/vulkan/specs/1.3/registry.html#_all_other_types)
From that perspective, it is essentially a black box resolver of types.
Specific types that are required for generating the API are later referenced
without a category.

**Example:** `<type category="include" name="windows.h"/>` is later followed by
`<type requires="windows.h" name="HWND"/>`. Note that `HWND` does
not have an associated category. Rather, that tag indicates that `HWND` is a
specific type needed from `windows.h`.

An includeType is the implementation of this concept in vk-gen. We will
register an `includeType` using the name in the specification. In
exceptions.json, that type will be annotated with `go:imports`
entries that fulfil the purpose of the referenced header files.

Additional `genericType` entires will be generated from static mappings under the
includeType. 

`vk_platform` is a special case that is required when referencing a number of C
primitive and conventional types (e.g., `float`, `uint32_t`, etc.) Those types
will be explicitly mapped to Go types in exceptions.json.

Consider a type like `HWND`: when that name appears the
`VkWin32SurfaceCreateInfoKHR` struct, we will reference back to the type
registry and find a `genericType` with that name. That generic type will have a
mapping showing that the name of that type in Go is `windows.HWND`. It will also
be annotated with a requirement for windows.h, which is the key of an
`includeType`. That `includeType` will have a go:imports annotation to import
`golang.org/x/sys/windows`. 

Note that there is no go:build directive here. That
build directive is pulled in by the extension that actually references the
struct and which has a platform guard in vk.xml.


An `include` map has entries that resolve to a platform guard, to a set of Go
imports, and mapping from C/Vulkan/platform types to Go types. Each type defined can also have
constants defined.

Include map fields:

* `platform` - A string matching a Vulkan platform name, which this include is
  limited to.
* `types` - A map containing keys that match types listed in vk.xml, with
  objects describing the types (see below).
* `go:imports` - an array of strings; each string indicating an import that is
  needed when this platform is included. For example, win32 requires
  "golang.org/x/sys/windows", as HWND in C/Vulkan maps to windows.Handle in Go.

Type objects:
* `go:type` - The name of the type to map to in Go
* `primitive` - boolean true if this is a primitive Go type that does not need
  be explicity declared or imported
* `constants` - A map with keys for Vulkan identifiers that should be defined or
  overridden; values are strings that will be exported to the generated code.

There is a quirk with mapping Go strings (and other array/slice types)
correctly. Vulkan has both char* strings (see VkApplicationInfo) and fixed
length char arrays (see VkPhysicalDeviceProperties). Plus, in C-style, the type
of a char* string is just char, and the pointer asterisk lives in text data
outside of the type node.

TODO how is that handled?

An includeType is a type required in Vulkan but defined outside of te API.
Primarily it encompases primitive types and OS or window-system specific types
(HWND or wl_display, for example).

Structurally, vk.xml declares a header file as an include type, and then
declares what types from that header will be used. The API also specifies
platforms that are supported. (For example, see the node specified by this
XPath query: `//platforms/platform[@name='win32']`.) Then, Vulkan extensions can
be linked to their supported platform (see for example
`//extension[@platform='win32']`).

Note that platform-specific included types, such as Windows' HWND or Vulkan's
VkWin32SurfaceCreateInfoKHR are *not* directly tied to a platform. Instead those types
are grouped into extensions that are guarded by platform. (See
`//extension[@name='VK_KHR_win32_surface']` for an example.) There is also a
meta-platform called vk_platform for primitive types and an exception from that
model for `int`, simply declared as `<type name="int">`.

Platforms in Vulkan are treated as feature sets in vk-gen, similar to the way
that Vulkan declares 1.0, 1.1 etc. as distinct groups of features. The platform
names are read from vk.xml and then extended from exceptions.json, primarily by
adding go:build tags.

`platform` map has keys for Vulkan platforms, and object entries containing the
following fields:
* `go:build` - A Go platform name for building
* `go:imports` - Any imports needed for that platform, in addition to any
  imports from `include` exceptions (see below)
* `comment` - Comments to add to the comment from vk.xml (if present)
* `!comment` - Comments regarding exceptions.json that will be ignored when the
  file is procesed

Getting back to an includeType, most of the information vk-gen needs is brought
in through exceptions.json. Primitive C types are mapped to primitive Go types
(where possible), and platform specific types are mapped to Go packages and
types. Include types, in some respects, are more like a feature set than a type,
as they are fundamentally a collection of other data types to create bindings
for.

### basetype

`basetype` in the specification designates non-enumerated (with one exception)
primitive types. The exceptions file currently also includes definitions for the
funcpointer types (PFN_*), mapping those to `unsafe.Pointer`.

The exception is VkBool32 and VK_TRUE/VK_FALSE. The XML file actually defines
those values as uint32_t, and not VkBool32. However, go-vk re-maps Bool32 as Go
bools in the public API, so developers can simply use `true` and `false`. To
avoid any confusion, `VK_TRUE` and `VK_FALSE` are marked as "!ignore" under the
uint32 enum values.

The `basetype` map allows the following entries:
* `comment` - Comments to add to the comment from vk.xml (if present) and
  automatic documentation link.
* `!comment` - Internal comments that will be ignored when the file is procesed
* `go:type` - Substitution to replace this entry type in the public API. Note
  that this is different than the underlying type. See Bool32 for an example.
* `go:translatePublic` - bare function to translate the internal representation
  to the public type. Function must accept a single parameter of the internal
  type and return the public type.
* `go:translateInternal` - bare function name to translate the public facing
  type to an internal representation. Function must accept a single parameter of
  the public type and return the internal type.
* `underlyingType` - type registry name designating the underlying type
  definition for this entry

  ### TODO

* ~~Aliases on commands, enums, structs, etc. are not handled. Required for Vulkan 1.1 and above~~
* ~~Handle nil pointers passed to commands (e.g. avoid calling Vulkanize)~~
* ~~`Goify` "output-only" structs from commands~~ - Everything gets Goify()
* ~~Commands output and filtering for OS/platform as needed - Dynamic load of library, avoiding Cgo whereever
  possible. Funnel everything through a singular (or small number) trampoline function that calls Cgo. Benchmarking is
  in order.~~
* ~~Flesh out the "static" portion of code: #defines, VK_VERSION etc.~~
* ~~Struct members: rename PNext to Next, handle both null-terminated strings and byte arrays as Go strings.~~
* Handle fixed size array members in struct: VkTransformMatrixKHR (integer), VkExtensionProperties (predefined const
  size) - Partial support. 
    * ~~single-dimension arrays in structs are supported, eg VkClearColorValue float[4]~~
    * VkTransformMatrixKHR has a [3][4] member, multi-dimensional arrays not currently handled
    * arrays as inputs to commands (vkCmdSetBlendConstants) not handled
* ~~Handle bool <-> Bool32 conversion (users should not have to be exposed to Bool32, right?)~~
* Handle feature and extension tags for output set. Should be able to say "output for version" or include/exclude
  extensions. Tags (specifically extensions) should be grouped into platform sets, for specific build-tag handling.
    * Feature is nominally supported, but some options should be added on the command line. Specifics TBD
    * Need to filter extensions to exclude (for now) provisional
    * ~~Need to segment (I think) platform extensions into platform groups, e.g., guard Win32 surface, Wayland surface, etc. functions
      with go:build tags~~
* ~~UpdateDescriptorSetWithTemplate quirk with byte* not being handled, causes compile error~~
* ~~Enumerate... functions need to query for results length and then allocate a slice, query again, then return the
  slice. Flagged in the registry as a pointer with a len specifier (an array), with optional=true, and the length
  paramter is specified as optional="false,true"~~
* Possible performance opt: Provide destination address for Vulkanize()...some (many) calls to that func are building
arrays, which then requires a dereference and copy of the struct into the array/slice location. Since the slice is
pre-allocated before the loop (not appended to), Vulkanize could build the output directly in the destination slice
memory, instead of on the stack and then forcing a copy.
* ~~Implement a feature set (core Vulkan functionality) -> category items (structs, commands, etc.), with required/implied
types automatically resolved.  Implied types means included by reference. For example, HWND is a platform type, but it
is never directly referenced in a platform specific way. It is only a member of VkWin32SurfaceCreateInfoKHR, which is
itself included through VK_KHR_win32_surface, a win32 specific extension.~~
    * Partial support; required types are automatically included in the Resolve() process, but selecting specific
      extensions is not yet available.
* ~~Handle nil slices in Vulkanize - eg. no instance layer names requested in InstanceCreateInfo causes nil pointer err~~
* ~~Handle nil pointers passed to Vulkanize, eg AllocationCallbacks~~
* Lots of code simplification is possible, especially when generating commands and structs. Need to determine what
  logic branches or assignments are dev legacies that never get executed (or get overwritten) in practice.

### API Contract for Resolve()

Resolve is a critical function of types, values, feature sets, etc. On a TypeDefiner, Resolve() must recursively call
Resolve on any types it depends on (e.g., the other it is defined by, or the type of each struct member), and it must
include that list of depended-on type names on return. If returned to another type, that parent type must merge the
return list with its own list and return that.

Resolve MUST return an empty set of required names if this instance was already resolved. Any type implementing Resolve MUST
NOT return itself as a required type.

For Features, each type's required list must be flagged as a required type for the feature. The Feature can assume that
those required types have already been resolved and can directly insert the names into its own map. We don't care if the
newly inserted entry (which will be a rare case) is visited later in the loop, because it will have already been
resolved and will return immediately with no further types required.



## Open Questions

### VkResult as error

VkResults are currently returned by value as the first return. VkResult does implement the error interface, and Go-style
would be to return an error interface as the last result. Moving it to the last return value is trivial, but direct comparison of
the result to error codes would require de-referencing the return value, and hard-coding the pointer on the back end, or
wrapping the result in a `vkerror` struct or similar.

As it stands now, code looks something like this:
``` go
r, instance := vk.CreateInstance(icInfo, nil)

if r != vk.SUCCESS {
    fmt.Printf("Failed to create Vulkan instance, error code was %s\n", r.Error()) 
    // you can also do r.String(), which returns the same value as Error()
} 
// vk.Result is just an int32. You could also switch r {...} to handle specific error codes.
```

Alternative A:
```go
instance, r := vk.CreateInstance(icInfo, nil)
if r != vk.SUCCESS { ... }
```
Alternative B:
```go
instance, err := vk.CreateInstance(icInfo, nil)
if err != nil {
  if err.Result() == vk.HOST_OUT_OF_MEMORY { ... }
}
```
Alt B above should probably get a special handling case to return vk.SUCCESS on a nil receiver (rather than panic/crash),
so you could also `switch(err.Result()) { ... }`

### Optimizations/Tuning Notes

Go-vk has NOT been profiled or optimized yet...the goal is to get the binding working and tested first. Listed here are
possible areas for optimization.

* Function dispatch is currently a map of int keys to lazy-eval structs, with the map being filled through init() at
  runtime. Since the map is static and will never be modified at runtime, all of the structs could just be hard-coded
  in a global var block to save the map lookup.
  * The structs do need to be global, not local to their command functions, because the dispatch is looking up and saving the
    function handle on first call.
* Use Vulkan's getInstanceProcAddress for symbols instead of the exports from the shared library. (I suspect those
  exported symbols just call that function internally, so it isn't clear what performance gain, if any, there will be.)
  * Likewise for getDeviceProcAddress, though that will require the user to provide the device they've obtained. It
    would, of course, be simpler if we used dispatchable handles as proc lookups by calling functions directly on the
    handles, though I think that makes end-user code a little less readable. See note below.
  * TBD, but I'd guess that a command fetched with getInstanceProcAddress, passed a device handle, simply calls
    getDeviceProcAddress behind the scenes, which just looks up an address in the device's dispatch table. Adding 1
    function call beyond the Cgo barrier is probably very little gain.

### Dispatchable Handles as Receivers?

Should dispatchable handles (Instance, Device, PhysicalDevice, Queue, CommandBuffer) be receivers for methods? Bindings
for some other languages, notably C++, use this approach and eliminate the handle from the parameter list. (Vulkan C++
also defines the "naked" function that takes a device handle as the first parameter, for compatability.) In this
scenario, go-vk could also attach the device-specific PFNs to the device handle, and then could be lazily
queried/evaluated for that device.

For example, vk.CreateDevice(...) returns a Device handle. While struct pointers are generally used as receivers, the Go
language allows any datatype to be used as a receiver, so you would write 
myDevice.CreateCommandPool(createInfo...), instead of vk.CreateCommandPool(myDevice, createInfo...).

A: Clarity and simplicity in code are guiding principles for Go. `vk.CreateCommandPool(device, ...)` is much more
explicit than `myDevice.CreateCommandPool`: there is no question that the first is a Vulkan API call, but the second
might not be obvious.

### VkAllocationCallbacks

Should we even include VkAllocationCallbacks in the binding? It is currently being generated automatically, but simply
will not work with Go function references (at least, not without a lot of background wiring). At the same time, you
won't have custom memory allocation or management in Go...you would already be working in C, C++, or Rust if that was a
requirement. Debug callbacks or allocation logging are a possible use case, but would still require the user to write
the callback in C, then call into their Go code.

A: The allocation callbacks struct is just an additional function parameter that will always be nil. Defer any decision
and leave it in place for the moment, in the interest of getting vk-gen and go-vk to be mostly feature complete.
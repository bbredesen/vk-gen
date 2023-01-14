# vk-gen Design

vk.xml schema documentation: [https://registry.khronos.org/vulkan/specs/1.3/registry.html]

## vk.xml and exceptions.json
There are three significant barriers to generating the Go-binding for Vulkan:

1) vk.xml includes a lot of C-preprocessor macros, included types from other
   headers, and C syntax for some bitmasks; all features that cannot be easily
   translated to Go in an automatic fashion.
1) Pointer types in C map to several different representations in Go. For
   example, a void pointer will generally be an unsafe.Pointer in Go, a char* is
   a string (but there are also fixed length char arrays in the spec), other
   pointers types are slices of structs, and a char** is a slice of strings.
1) There are a small number of Vulkan commands and structs whose semantics are
   inconsistent with the rest of the library.

Together, this means that we need a structured set of exceptions or overrides
for the tool to read, implemented as exceptions.json. This reduces
maintainability and potentially requires new exceptions be added on API updates.

## Vulkan API Specification Quirks

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
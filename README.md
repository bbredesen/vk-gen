# vk-gen

vk-gen is a tool used to create Go bindings for the Vulkan graphics API. It uses the Vulkan XML specification to
generate all type definitions and native function calls for the API. It generates the code for
[go-vk](https://github.com/bbredesen/go-vk), but it can just as well be used to create a modified binding set in your
own projects (for example, excluding certain vendor extensions, including beta extensions, or to generate code from a
specific version of the Vulkan headers). 

## Basic Usage

**You do not need to install this tool to use go-vk in a project.** Install if you need to generate for a specific
version of the API or want to produce a binding using only a subset of Vulkan.

Install: `go install github.com/bbredesen/vk-gen@latest`

Download the latest Vulkan registry file: `curl https://raw.githubusercontent.com/KhronosGroup/Vulkan-Headers/main/registry/vk.xml > vk.xml`

(Or, replace "main" in the URL above with the tagged version you want to generate against: e.g., 
`https://raw.githubusercontent.com/KhronosGroup/Vulkan-Headers/v1.2.203/registry/vk.xml` for the last version 1.2 specification.)

Run the tool: `vk-gen`

Use `-inFile` to specify a registry filename or path (defaults to `./vk.xml`)

Use `-outDir` to specify the destination folder for writing go-vk files (defaults to `./vk/`)

The `_static_include` folder in this repository contains static template files that are copied directly into the output
folder. These files are embeded in the binary and then written to the output, but are not evaluated or otherwise used uin this tool. Go tooling should ignore this folder because of the underscore prefix, but you can safely ignore linters or other warnings that arise.

## exceptions.json

`exceptions.json` is also embedded, allowing you to `go install` and run without having a local copy of this file.

There are a number of datatypes and values in vk.xml which need special handling, frequently because the spec uses
C data type formats or types that don't translate 1-to-1 to Go's type system. While we could probably work
around many of them by parsing the C code in the XML file, it is much simpler to set these exceptions in a separate file
with a standard format.

NOTE: There are a number of "legacy" entries in this file left over from development, but which are now unused. A future
issue/PR will clean this up, but they don't hurt anything at the moment.

### union

* `go:internalSize` - Go has no notion of union types. This field allows you to specify a size for the public
  to internal translation result. By default, vk-gen will use the size of the first member in the union, but that is
  not necessarily the largest member. This value must be a string and is copied to an array declaration. It can be
  anything that resolves to a constant in Go, though most typically it will be an integer value (represented as a
  string). The value should be the aligned (?) data size in bytes of the largest member of the union. 
* `go:selector` and `go:selections` - Used for returned unions whose registry entry has no `selector` attribute (only
  `VkPerformanceCounterResultKHR` at present). `go:selector` is the registry name of the enum type that decides which
  member is valid, and `go:selections` maps each union member's registry name to the enum value that selects it. These
  unions get a `Goify(selector)` and an exported `<Name>FromBytes(buf, selector)` function. Unions that do have a
  `selector`/`selection` in vk.xml are handled automatically.

## Testing against Vulkan registry versions

`matrix/` runs vk-gen against pinned `vk.xml` files (the last release of each minor version, plus the current 1.4
revision) and checks that the generated package builds and passes its tests; see [matrix/README.md](matrix/README.md).

    go test -count=1 ./matrix   # needs goimports and stringer in PATH or GOPATH/bin

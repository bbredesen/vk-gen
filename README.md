# vk-gen

vk-gen is a tool used to create Go bindings for the Vulkan graphics API. It uses the Vulkan XML specification to
generate all type definitions and native function calls for the API. It generates the code for
[go-vk](https://github.com/bbredesen/go-vk), but it can just as well be used to create a modified binding set in your
own projects (for example, excluding certain vendor extensions, incluidng beta extensions, or to generate code from a
specific version of the Vulkan headers). 

## Basic Usage

**You do not need to install this tool to use go-vk in a project.** Install if you need to generate for a specific
version of the API or want to produce a binding using only a subset of Vulkan.

Install: `go install github.com/bbredesen/vk-gen@latest`

Download the latest registry file: `curl https://raw.githubusercontent.com/KhronosGroup/Vulkan-Headers/main/registry/vk.xml > vk.xml`

Run the tool: `vk-gen`

Use `-inFile` to specify a registry filename or path (defaults to `./vk.xml`)

Use `-outDir` to specify the destination folder for writing go-vk files (defaults to `./vk/`)

The `static_include` folder in this repository contains static template files that are copied directly into the output
folder. These files are directly copied to the output, but are not evaluated or compiled into this tool. If using the Go
language server, you can set `-static_include` in your `directoryFilters` setting. See
(https://github.com/golang/tools/blob/master/gopls/doc/settings.md) for details.

## exceptions.json

There are a number of datatypes and values in vk.xml which need special handling, frequently because the spec uses
C data type formats or types that don't translate 1-to-1 to Go's type system. While we could probably work
around many of them by parsing the C code in the XML file, it is much simpler to set these exceptions in a separate file
with a standard format.

NOTE: There are a number of "legacy" entries in this file left over from development, but which are now unused. A future
issue/PR will clean this up, but they don't hurt anything at the moment.

### union

* `go:internalSize` - Go has no notion of union types. This field allows you to specify a size for the public
  to internal translation result. By default, vk-gen will use the size of the first member in the union, but that is
  not neccesarily the largest member. This value must be a string and is copied to an array declaration. It can be
  anything that resolves to a constant in Go, though most typically it will be an integer value (represented as a
  string). The value should be the aligned (?) data size in bytes of the largest member of the union. 

## Development and Design Notes

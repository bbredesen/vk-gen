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

Download the latest registry file: `curl
https://raw.githubusercontent.com/KhronosGroup/Vulkan-Headers/main/registry/vk.xml > vk.xml`

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

### union

* `go:internalSize` - Go has no notion of union types. This field allows you to specify a size for the public
  to internal translation result. By default, vk-gen will use the size of the first field in the union, but that is
  frequently not the largest field. This value must be a string and is copied to an array declaration. It can be
  anything that resolves to a constant in Go, though most typically it will be an integer value (represented as a
  string). The value should be the aligned (?) data size in bytes of the largest member of the union. 

## Development and Design Notes
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
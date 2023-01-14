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

## TODO

* `Goify` "output-only" structs from commands
* Commands output and filtering for OS/platform as needed - ~~Dynamic load of library,~~ avoiding Cgo whereever
  possible. Funnel everything through a singular (or small number) trampoline function that calls Cgo. Benchmarking is
  in order.
* ~~Flesh out the "static" portion of code: #defines, VK_VERSION etc.~~
* ~~Struct members: rename PNext to Next, handle both null-terminated strings and byte arrays as Go strings.~~
* Handle fixed size array members in struct: VkTransformMatrixKHR (integer), VkExtensionProperties (predefined const
  size) - Partial support. 
    * single-dimension arrays in structs are supported, eg VkClearColorValue float[4] 
    * VkTransformMatrixKHR has a [3][4] member, multi-dimensional arrays not currently handled
    * arrays as inputs to commands (vkCmdSetBlendConstants) not handled
* ~~Handle bool <-> Bool32 conversion (users should not have to be exposed to Bool32, right?)~~
* Handle feature and extension tags for output set. Should be able to say "output for version" or include/exclude
  extensions. Tags (specifically extensions) should be grouped into platform sets, for specific build-tag handling.
    * Feature is nominally supported, but some options should be added on the command line. Specifics TBD
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

## Open Questions

### Dispatchable Handles as Receivers?

Should dispatchable handles (Instance, Device, PhysicalDevice, Queue, CommandBuffer) be receivers for methods? Bindings
for some other languages, notably C++, use this approach and eliminate the handle from the parameter list. (Vulkan C++
also defines the "naked" function that takes a device handle as the first parameter, for compatability.) In this
scenario, go-vk could also attach the device-specific PFNs to the device handle, and then could be lazily
queried/evaluated for that device.

For example, vk.CreateDevice(...) returns a Device handle. While struct pointers are generally used as a method
receiver, Go language allows any datatype to be used as a receiver, so you would write
myDevice.CreateCommandPool(createInfo...), instead of vk.CreateCommandPool(myDevice, createInfo...).

A: Clarity and simplicity in code are guiding principles for Go. `vk.CreateCommandPool(device, ...)` is much more
explicit than `myDevice.CreateCommandPool`: there is no question that the first is a Vulkan API call, but the second
might not be obvious.

### VkAllocationCallbacks

Should we even include VkAllocationCallbacks in the binding? It is currently being generated automatically, but simply
will not work with Go function references (at least, not without a lot of background wiring). At the same time, you
won't have custom memory allocation or management in Go...you would already be working in C, C++, or Rust if that was a
requirement. Debug callbacks are a possible use case, but would still require the user to write the callback in C, then
call into their Go code.

A: The allcation callbacks struct is just an additional function parameter that will always be nil. Leave it in place
for the moment in the interest of getting vk-gen and go-vk to be mostly feature complete. It can be removed later.

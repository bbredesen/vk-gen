# Registry matrix tests

These tests run vk-gen against pinned versions of the Vulkan registry (`vk.xml`) and check that each one produces a Go
package that builds and passes its tests in matrix_test.go, including a diff test against the appropriate "golden" copy
of go-vk stored under this directory.

vk-gen is meant to work with any published version of the specification, so
that someone targeting a limited Vulkan implementation can generate against the version they need. This is the check
that keeps it that way.

The matrix pins the last release of each minor version, plus the current 1.4 revision:

| Version | Source |
|---|---|
| 1.0.69-core | Vulkan-Docs `v1.0.69-core` (`src/spec/vk.xml`); Vulkan-Headers has no tag this old |
| 1.1.130, 1.2.203, 1.3.302, 1.4.366 | Vulkan-Headers `v<version>` (`registry/vk.xml`) |

## Running

```sh
go test -count=1 ./matrix            # all versions, about 30 seconds
go test -count=1 ./matrix -run 'RegistryMatrix/1.2'   # one version
go test -count=1 ./matrix -v         # shows each stage
```

The test builds and runs vk-gen as a separate program, so Go's test cache cannot see that vk-gen's source
changed and will report a stale pass. Use `-count=1` to force a test run.

Testing needs `goimports` and `stringer` (in `PATH` or `GOPATH/bin`) and a C compiler, because the generated package uses cgo.
Without the tools the test skips locally; it fails if `CI` or `VK_MATRIX_REQUIRE_TOOLS` is set. The generated package is
built for the host OS only, since cgo cannot be cross-compiled without a cross C compiler. The Linux platform files (xcb, xlib, wayland) are therefore compiled only by the `ubuntu-latest` job in CI. The first run may download
`golang.org/x/sys`.

## The Go 1.18 floor

go-vk supports Go 1.18 and later. Nothing newer is used by the library; the static test files avoid newer builtins too.
Two checks enforce it, because they catch different things:

- The generated `go.mod` says `go 1.18`, so every run, on every OS, rejects newer **language** features.
- A real 1.18 toolchain catches newer **standard library** functions (for example `strings.CutPrefix`, added in 1.20),
  which the go directive does not. Set `VK_MATRIX_GO_TOOLCHAIN=go1.18.10` to compile, vet and test the generated packages
  with it; the generator and stringer still run on your current Go. CI does this in the `go-floor` job.

```sh
VK_MATRIX_GO_TOOLCHAIN=go1.18.10 go test -count=1 -run TestRegistryMatrix ./matrix
```

## Layout

```
matrix_test.go                  the tests
testdata/
  registry/
    vk-<version>.xml            pinned registry files, committed so tests never need the network
    SHA256SUMS                  checked by TestRegistryChecksums
    fetch.sh                    downloads the pinned files; only run it to add or re-pin a version
  golden/<version>/             expected output for a version (see below)
  excluded_static_tests.json    static test files removed for versions that predate what they use
```

The tests are in their own package, and not in the root `package main`, because vk-gen parses its flags in `init()` and
would break `go test` flag handling.

## Stages

`TestRegistryMatrix` runs these in order for each `testdata/registry/vk-<version>.xml`. A version stops at its first
failing stage.

1. **generate**: vk-gen exits 0, logs nothing at error level or above (vk-gen reports most problems with `logrus.Error`
   and carries on, so the exit status is not enough), and a second run produces byte-identical files.
   The registry file is passed by a fixed relative name because the name is written into every generated file's header.
2. **gogenerate**: `go generate ./...` runs stringer on the output.
3. **build**: `go build ./...`. The output is written as a standalone module (`github.com/bbredesen/go-vk`) that declares
   `go 1.18` and requires `golang.org/x/sys` v0.5.0, the oldest versions go-vk supports. The `go 1.18` line makes the
   compiler reject newer language features (builtin `min`, `max`, `clear`, range over an integer) on any toolchain.
4. **vet**: `go vet -unsafeptr=false ./...`. The unsafeptr check is off because the bindings pass pointers to C by design.
   This also compiles the static test files.
5. **test**: `go test ./...` runs the static tests from `_static_include` against a fake Vulkan library.
6. **golden**: the output, including the stringer files, is compared to `testdata/golden/<version>`. Go files are compared
   as tokens (comments included), not as text, so layout is ignored. gofmt's alignment of struct literals changed between Go
   1.26 and 1.27, and a golden recorded with one would otherwise fail on the other. Re-recording with a different Go version
   can still churn whitespace in `git diff`; that is harmless. The files copied
   from `_static_include`, and the test's `go.mod` and `go.sum`, are left out of the comparison because they do not depend on
   the registry. A version with no golden directory skips this stage, so it is a smoke test only.

`TestRegistryChecksums` checks the committed registry files against `SHA256SUMS`, so a pinned file cannot change unnoticed.

## Excluded static tests

The files in `_static_include/*_test.go` are one unit (they share helpers) and use specific extensions, for example
`VK_EXT_debug_utils` and `SetDeviceMemoryPriorityEXT`. They cannot compile against a registry that predates those, which
is correct and not a generator bug. `excluded_static_tests.json` lists the test files removed from a version's output
before the vet and test stages, with the reason. Do not use it to hide a failure in a version that has the API.

## Adding a registry version

1. Add the version to `testdata/registry/fetch.sh` and run it. It downloads the files and rewrites `SHA256SUMS`. Use the
   tagged file, not a branch, as described in the vk-gen README (for example
   `https://raw.githubusercontent.com/KhronosGroup/Vulkan-Headers/v1.4.366/registry/vk.xml`). The file must be named
   `vk-<version>.xml`.
2. Run `go test -count=1 ./matrix -run 'RegistryMatrix/<version>' -v`. Without a golden it runs the smoke stages
   (generate through test). To try a version without touching `fetch.sh`, drop the file into `testdata/registry/` and
   add its checksum to `SHA256SUMS`.
3. If a stage fails, fix the generator. A pinned version must pass every stage. If the failure is only because the 
   version predates an API that the static tests use, see "Excluded static tests".
4. When the version passes every stage, record the golden:

   ```sh
   go test -count=1 ./matrix -update -run 'RegistryMatrix/<version>'
   ```

   `-update` rewrites `testdata/golden/<version>` for each version that passes through the golden stage. Review
   `git diff` before committing; the golden is the expected output, and each change to it should be one you meant to make.
5. Commit the `.xml` file, `SHA256SUMS`, and the golden directory (about 1 to 4 MB).

## Changing the generator

A generator change that alters output shows up as a golden diff. If it is intended, run `go test -count=1 ./matrix -update`
and commit the changed golden files with the generator change, so the diff shows what the change did to every version.

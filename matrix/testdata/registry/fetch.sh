#!/bin/sh
# Downloads the pinned Vulkan registry files and records their checksums in SHA256SUMS. The files are committed so that
# tests never need the network; run this only to add or re-pin a version. Add new versions to the list below, then
# commit the .xml files and SHA256SUMS.
#
# Registry files come from the tagged Vulkan-Headers repo (see the vk-gen README). Vulkan-Headers has no tags before
# v1.1.x, so 1.0 comes from the last Vulkan-Docs core-only tag, where the registry lived in src/spec/.
set -eu
cd "$(dirname "$0")"

B=https://raw.githubusercontent.com/KhronosGroup

fetch() { # version url
	echo "fetching $1"
	curl -sfL "$2" -o "vk-$1.xml"
}

fetch 1.0.69-core "$B/Vulkan-Docs/v1.0.69-core/src/spec/vk.xml"
fetch 1.1.130 "$B/Vulkan-Headers/v1.1.130/registry/vk.xml"
fetch 1.2.203 "$B/Vulkan-Headers/v1.2.203/registry/vk.xml"
fetch 1.3.302 "$B/Vulkan-Headers/v1.3.302/registry/vk.xml"
fetch 1.4.365 "$B/Vulkan-Headers/v1.4.365/registry/vk.xml"

shasum -a 256 vk-*.xml > SHA256SUMS

package def

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
)

// A command that was promoted to core, or to another extension, keeps the old name as an alias in the registry
// (vkCmdBeginRenderingKHR for vkCmdBeginRendering). An alias has the same signature, so go-vk declares it as a Go
// variable holding the target function. At run time, though, the two names are different Vulkan commands: a device that
// supports the extension but not the core version offers only the old name through vkGetDeviceProcAddr. The binding
// therefore records each printed alias against its target, and command resolution tries the alias names when the target's
// own name is not found (see resolveSlow in static_common.go).

// commandAliasNames maps a command's registry name to the registry names of the aliases printed for it.
var commandAliasNames = map[string][]string{}

// registerCommandAlias records an alias command against the command it ultimately refers to.
func registerCommandAlias(alias *commandType) {
	target := alias.resolvedAliasType
	for depth := 0; depth < 16; depth++ {
		next, ok := target.(*commandType)
		if !ok || !next.IsAlias() {
			break
		}
		target = next.resolvedAliasType
	}

	names := commandAliasNames[target.RegistryName()]
	if !slices.Contains(names, alias.registryName) {
		commandAliasNames[target.RegistryName()] = append(names, alias.registryName)
	}
}

// WriteCommandAliasFile generates command_aliases.go into dir, which declares commandAliases for the package. The file is
// written even when there are no aliases, because static_common.go refers to the variable. header is the generated-code
// header that begins every file, ending in the package clause.
func WriteCommandAliasFile(dir, header string) {
	targets := make([]string, 0, len(commandAliasNames))
	for target := range commandAliasNames {
		targets = append(targets, target)
	}
	slices.Sort(targets)

	sb := &strings.Builder{}
	sb.WriteString(header)
	sb.WriteString("// commandAliases maps a command to the other names Vulkan knows it by, in the order they are tried when the\n")
	sb.WriteString("// command's own name cannot be found.\n")
	sb.WriteString("var commandAliases = map[string][]string{\n")
	for _, target := range targets {
		names := slices.Clone(commandAliasNames[target])
		slices.Sort(names)
		quoted := make([]string, len(names))
		for i, n := range names {
			quoted[i] = fmt.Sprintf("%q", n)
		}
		fmt.Fprintf(sb, "%q: {%s},\n", target, strings.Join(quoted, ", "))
	}
	sb.WriteString("}\n")

	src, err := format.Source([]byte(sb.String()))
	if err != nil {
		logrus.WithField("error", err).Fatal("Could not format command alias source")
	}
	if err := os.WriteFile(filepath.Join(dir, "command_aliases.go"), src, 0666); err != nil {
		logrus.WithField("error", err).Fatal("Could not write command_aliases.go")
	}

	logrus.Infof("Generated %d command alias entries", len(targets))
}

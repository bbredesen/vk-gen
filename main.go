package main

import (
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/bbredesen/vk-gen/def"
	"github.com/bbredesen/vk-gen/feat"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var (
	inFileName, outDirName string
	apiName                string
	platformTargets        string
	separatedPlatforms     []string
)

func init() {
	flag.StringVar(&inFileName, "inFile", "vk.xml", "Vulkan XML registry file to read")
	flag.StringVar(&outDirName, "outDir", "vk", "Directory to write go-vk output to")
	flag.StringVar(&apiName, "api", "vulkan", "API to generate against; possible values include 'vulkan' and 'vulkansc'")
	flag.StringVar(&platformTargets, "platform", "win32,macos,metal", "Comma-separated list of platforms to generate for; this looks at the Vulkan name, not the GOOS name for the platform")

	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{ForceColors: true})
}

func main() {

	_, err := os.Stat(outDirName)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.Mkdir(outDirName, 0777|fs.ModeDir); err != nil {
				logrus.WithField("error", err).
					Fatal("Could not create output directory")
			} else {
				logrus.WithField("directory", outDirName).
					Info("Output directory created")
			}
		} else {
			logrus.WithField("error", err).
				Fatal("Could not stat output directory")
		}
	}

	f, err := os.Open(inFileName)
	if err != nil {
		logrus.WithField("error", err).
			WithField("filename", inFileName).
			Fatal("Could not open Vulkan registry file")
	}
	defer f.Close()

	// Split and clean up the platform targets string into a slice of platform names
	separatedPlatforms = strings.Split(platformTargets, ",")
	for i, s := range separatedPlatforms {
		separatedPlatforms[i] = strings.TrimSpace(s)
	}
	separatedPlatforms = slices.DeleteFunc(separatedPlatforms, func(s string) bool { return s == "" })
	separatedPlatforms = slices.Compact(separatedPlatforms)

	if len(separatedPlatforms) == 1 && separatedPlatforms[0] == "" {
		logrus.Info("Generating core Vulkan only; no platform specific extensions will be available!")
	} else {
		logrus.WithField("platforms", separatedPlatforms).Infof("Found %d platforms to generate for", len(separatedPlatforms))
	}
	xmlDoc, err := xmlquery.Parse(f)
	if err != nil {
		logrus.WithField("filename", inFileName).
			WithField("error", err).
			Fatal("Could not parse XML from the provided file")
	}

	exceptionsBytes, err := os.ReadFile("exceptions.json")
	if err != nil {
		logrus.WithField("error", err).
			Fatal("Could not parse json from exceptions.json")
	}

	jsonDoc := gjson.ParseBytes(exceptionsBytes)
	globalTypes := make(def.TypeRegistry)
	globalValues := make(def.ValueRegistry)

	// pm := def.ReadPlatformsFromXML(xmlDoc)
	// def.ReadPlatformExceptionsFromJSON(jsonDoc, pm)

	for tc := def.CatNone; tc < def.CatMaximum; tc++ {
		xml, json := tc.ReadFns()
		if xml != nil {
			xml(xmlDoc, globalTypes, globalValues, apiName)
		}
		if json != nil {
			json(jsonDoc, globalTypes, globalValues)
		}
	}

	platforms := make(feat.PlatformRegistry)
	// static platform
	platforms[""] = feat.NewGeneralPlatform()
	for _, n := range xmlquery.Find(xmlDoc, "//platforms/platform") {
		plat := feat.NewPlatformFromXML(n)
		platforms[plat.Name()] = plat
	}
	jsonDoc.Get("platform").ForEach(func(key, value gjson.Result) bool {
		if key.String() == "!comment" {
			return true
		}
		if slices.Contains(separatedPlatforms, key.String()) {
			r := feat.NewOrUpdatePlatformFromJSON(key.String(), value, platforms[key.String()])
			platforms[r.Name()] = r
		}
		return true
	})

	for _, name := range separatedPlatforms {
		if _, ok := platforms[name]; !ok {
			logrus.WithField("platform", name).Fatal("Requested platform is not defined in vk.xml or exceptions.json")
		}
	}

	vk1_0 := feat.ReadFeatureFromXML(xmlquery.FindOne(xmlDoc, "//feature[@name='VK_VERSION_1_0']"), globalTypes, globalValues)
	vk1_1 := feat.ReadFeatureFromXML(xmlquery.FindOne(xmlDoc, "//feature[@name='VK_VERSION_1_1']"), globalTypes, globalValues)
	vk1_2 := feat.ReadFeatureFromXML(xmlquery.FindOne(xmlDoc, "//feature[@name='VK_VERSION_1_2']"), globalTypes, globalValues)
	vk1_0.MergeWith(vk1_1)
	vk1_0.MergeWith(vk1_2)

	// Manually include external types
	vk1_0.MergeIncludeSet(globalTypes.SelectCategory(def.CatExternal))

	for _, platName := range separatedPlatforms {
		xpath := fmt.Sprintf("//extension[@platform='%s']", platName)
		for _, extNode := range xmlquery.Find(xmlDoc, xpath) {
			ext := feat.ReadExtensionFromXML(extNode, globalTypes, globalValues)
			platforms[ext.PlatformName()].IncludeExtension(ext)
		}
	}

	// "Core" extensions
	extQueryString := fmt.Sprintf("//extension[not(@platform) and @supported = '%s']", apiName)
	for _, extNode := range xmlquery.Find(xmlDoc, extQueryString) {
		ext := feat.ReadExtensionFromXML(extNode, globalTypes, globalValues)
		platforms[""].IncludeExtension(ext)
	}

	vk1_0.MergeWith(platforms[""].GeneratePlatformFeatures())

	vk1_0.Resolve(globalTypes, globalValues)

	goimportsPath, err := findGoimports()
	if err != nil {
		logrus.
			WithField("error", err.Error()).
			Error("Could not find goimports")
	}

	for tc, reg := range vk1_0.FilterByCategory() {
		if tc == def.CatHandle {
			// Special case...VK_NULL_HANDLE is included by vk.xml as a type, not an enum. vk-gen treats it as a
			// ValueDefiner, so it must be manually added to the feature registry.
			globalValues["VK_NULL_HANDLE"].Resolve(globalTypes, globalValues)
			reg.ResolvedTypes["VK_DEFINE_HANDLE"].PushValue(globalValues["VK_NULL_HANDLE"])
		}

		printCategory(tc, reg, nil, goimportsPath)
	}

	for pName, plat := range platforms {
		if pName == "" {
			continue
		} else if plat.GoBuildTag == "!ignore" {
			logrus.WithField("platform", pName).Warn("Requested platform is marked '!ignore' in exceptions.json; skipping")
			continue
		}

		pf := plat.GeneratePlatformFeatures()
		pf.Resolve(globalTypes, globalValues)

		for tc, reg := range pf.FilterByCategory() {
			printCategory(tc, reg, plat, goimportsPath)
		}
	}

	copyStaticFiles()

}

const fileHeader string = "// Code generated by vk-gen from %s. DO NOT EDIT.\n\npackage vk\n\n" // fix doc/issue-1

func printCategory(tc def.TypeCategory, fc *feat.Feature, platform *feat.Platform, goimportsPath string) {
	if tc == def.CatInclude {
		return
	}

	reg := fc.ResolvedTypes

	if len(reg) == 0 && len(fc.ResolvedValues) == 0 {
		return
	}

	filename := strings.ToLower(strings.TrimPrefix(tc.String(), "Cat"))
	if platform != nil {
		filename = filename + "_" + platform.Name()
	}

	outpath := fmt.Sprintf("%s/%s", outDirName, filename+".go")

	f, err := os.Create(outpath)
	if err != nil {
		logrus.WithField("error", err).Fatal("Could not create output file")
	}
	// explicit f.Close() below; not deferred because the file must be written to disk before goimports is run

	if platform != nil && platform.GoBuildTag != "" && tc != def.CatEnum && tc != def.CatBitmask {
		fmt.Fprintf(f, "//go:build %s\n", platform.GoBuildTag)
	}

	fmt.Fprintf(f, fileHeader, inFileName)

	if platform != nil && len(platform.GoImports) > 0 {
		fmt.Fprintf(f, "import (\n")
		for _, i := range platform.GoImports {
			fmt.Fprintf(f, "\"%s\"", i)
		}
		fmt.Fprintf(f, ")\n")
	}

	types := make([]def.TypeDefiner, 0, len(reg))
	for k, v := range reg {
		_ = k
		types = append(types, v)
		v.AppendValues(fc.ResolvedValues[v.RegistryName()])
		delete(fc.ResolvedValues, v.RegistryName())
	}

	sort.Sort(def.ByName(types))
	def.WriteStringerCommands(f, types, tc, filename)

	importMap := make(def.ImportMap)
	for _, t := range types {
		t.RegisterImports(importMap)
	}
	if len(importMap) > 0 {
		keys := importMap.SortedKeys()
		fmt.Fprint(f, "import (\n")
		for _, k := range keys {
			fmt.Fprintf(f, "  \"%s\"\n", k)
		}
		fmt.Fprintln(f, ")")
		fmt.Fprintln(f)
	}

	printTypes(f, types)
	printLooseValues(f, fc.ResolvedValues)

	f.Close()

	logrus.WithField("file", filename+".go").Debug("Running goimports")

	cmd := exec.Command(goimportsPath, "-w", outpath)
	e := &strings.Builder{}
	cmd.Stderr = e

	goimpErr := cmd.Run()
	if goimpErr != nil {
		logrus.
			WithField("path", outpath).
			WithField("error", goimpErr.Error()).
			WithField("goimports output", e.String()).
			Error("Failed to format source file")
	}

}

func printTypes(w io.Writer, types []def.TypeDefiner) {
	contentBuf := &strings.Builder{}

	for _, v := range types {
		if strings.HasPrefix(v.PublicName(), "!") {
			continue
		}

		v.PrintPublicDeclaration(contentBuf)
		v.PrintInternalDeclaration(contentBuf)
	}

	fmt.Fprint(w, contentBuf.String())

}

func printLooseValues(w io.Writer, valsByTypeName map[string]def.ValueRegistry) {
	// sort and refactored for cleanup/issue-3
	keys := slices.Sorted(maps.Keys(valsByTypeName))

	for _, k := range keys {
		vr := valsByTypeName[k]
		// Values will be sorted by const name for extension names/spec versions, and by value for typed consts
		allValues := make([]def.ValueDefiner, 0, len(vr))
		for _, val := range vr {
			allValues = append(allValues, val)
		}

		sort.Sort(def.ByValue(allValues))

		if k == "" {
			fmt.Fprint(w, "// Extension names and versions\n")
		} else {
			fmt.Fprintf(w, "// Platform-specific values for %s\n", k)
		}

		fmt.Fprintf(w, "const (\n")

		for _, val := range allValues {
			val.PrintPublicDeclaration(w)
		}
		fmt.Fprintf(w, ")\n\n")
	}
}

func copyStaticFiles() {
	logrus.Info("Copying static files")
	source := "static_include"

	// Naive solution from https://stackoverflow.com/questions/51779243/copy-a-folder-in-go
	var err error = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		var relPath string = strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(filepath.Join(outDirName, relPath), 0777)
		} else {
			var data, err1 = os.ReadFile(filepath.Join(source, relPath))
			if err1 != nil {
				return err1
			}
			return os.WriteFile(filepath.Join(outDirName, relPath), data, 0666)
		}
	})

	if err != nil {
		logrus.WithField("error", err).Fatal("Could not copy static files")
	}
}

func findGoimports() (path string, err error) {
	// goimports is probably in GOPATH which may not be in the user's PATH
	goPath := os.Getenv("GOPATH")
	if goPath == "" {
		// If GOPATH is not set, use go's default
		goPath = build.Default.GOPATH
	}

	// There may be multiple paths, so split and add "/bin" to each
	paths := strings.Split(goPath, string(os.PathListSeparator))
	goPath = ""
	for _, path := range paths {
		goPath += fmt.Sprintf("%s%sbin%s", path, string(os.PathSeparator), string(os.PathListSeparator))
	}

	// Add PATH paths to the end
	goPath += os.Getenv("PATH")

	os.Setenv("PATH", goPath)

	return exec.LookPath("goimports")
}

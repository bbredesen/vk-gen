package main

import (
	"flag"
	"fmt"
	"go/build"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
	"github.com/bbredesen/vk-gen/def"
	"github.com/bbredesen/vk-gen/feat"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var (
	inFileName, outDirName string
	platformTargets        string
	separatedPlatforms     []string
	useTemplates           bool
)

func init() {
	flag.StringVar(&inFileName, "inFile", "vk.xml", "Vulkan XML registry file to read")
	flag.StringVar(&outDirName, "outDir", "vk", "Directory to write go-vk output to")
	flag.StringVar(&platformTargets, "platform", "win32", "Comma-separated list of platforms to generate for")

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
		}
	}

	f, err := os.Open(inFileName)
	if err != nil {
		logrus.WithField("error", err).
			WithField("filename", inFileName).
			Fatal("Could not open Vulkan registry file", inFileName, err)
	}
	defer f.Close()

	separatedPlatforms = strings.Split(platformTargets, ",")
	if len(separatedPlatforms) == 0 {
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
	_ = jsonDoc
	globalTypes := make(def.TypeRegistry)
	globalValues := make(def.ValueRegistry)

	pm := def.ReadPlatformsFromXML(xmlDoc)
	def.ReadPlatformExceptionsFromJSON(jsonDoc, pm)

	for tc := def.CatNone; tc < def.CatMaximum; tc++ {
		xml, json := tc.ReadFns()
		if xml != nil {
			xml(xmlDoc, globalTypes, globalValues)
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
		r := feat.NewOrUpdatePlatformFromJSON(key.String(), value, platforms[key.String()])
		platforms[r.Name()] = r
		return true
	})

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
	for _, extNode := range xmlquery.Find(xmlDoc, "//extension[not(@platform) and @supported='vulkan']") {
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

	commandCount := 0

	for tc, reg := range vk1_0.FilterByCategory() {
		if tc == def.CatHandle {
			// Special case...VK_NULL_HANDLE is included by vk.xml as a type, not an enum. vk-gen treats it as a
			// ValueDefiner, so it must be manually added to the feature registry.
			globalValues["VK_NULL_HANDLE"].Resolve(globalTypes, globalValues)
			reg.ResolvedTypes["VK_DEFINE_HANDLE"].PushValue(globalValues["VK_NULL_HANDLE"])
		}

		printCategory(tc, reg, nil, 0, goimportsPath)
		if tc == def.CatCommand {
			commandCount += len(reg.ResolvedTypes)
		}

	}

	for pName, plat := range platforms {
		if pName == "" {
			continue
		}

		pf := plat.GeneratePlatformFeatures()
		pf.Resolve(globalTypes, globalValues)

		for tc, reg := range pf.FilterByCategory() { 
			printCategory(tc, reg, plat, commandCount, goimportsPath)
			if tc == def.CatCommand {
				commandCount += len(reg.ResolvedTypes)
			}
		}
	}

	copyStaticFiles()

}

const fileHeader string = "// Code generated by go-vk from %s at %s. DO NOT EDIT.\n\npackage vk\n\n" // fix doc/issue-1

func printCategory(tc def.TypeCategory, fc *feat.Feature, platform *feat.Platform, startingCount int, goimportsPath string) {
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

	f, _ := os.Create(outpath)
	// explicit f.Close() below; not deferred because the file must be written to disk before goimports is run

	if platform != nil && platform.GoBuildTag != "" {
		fmt.Fprintf(f, "//go:build %s\n", platform.GoBuildTag)
	}

	fmt.Fprintf(f, fileHeader, inFileName, time.Now())

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

	printTypes(f, types, fc.ResolvedValues, startingCount)
	printLooseValues(f, fc.ResolvedValues)

	f.Close()

	logrus.WithField("file", filename+".go").Info("Running goimports")

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

func printTypes(w io.Writer, types []def.TypeDefiner, vals map[string]def.ValueRegistry, globalOffset int) {
	globalBuf := &strings.Builder{}
	initBuf := &strings.Builder{}
	contentBuf := &strings.Builder{}

	for i, v := range types {
		if strings.HasPrefix(v.PublicName(), "!") {
			continue
		}

		v.PrintGlobalDeclarations(globalBuf, i+globalOffset, i == 0)

		v.PrintPublicDeclaration(contentBuf)
		v.PrintInternalDeclaration(contentBuf)

		v.PrintFileInitContent(initBuf) // Intentionally called after public declaration, which may do some processing needed for file init()
	}

	if globalBuf.Len() > 0 {
		fmt.Fprintf(w, "const (\n")
		fmt.Fprint(w, globalBuf.String())
		fmt.Fprintf(w, ")\n\n")
	}

	if initBuf.Len() > 0 {
		fmt.Fprint(w, "func init() {\n")
		fmt.Fprint(w, initBuf.String())
		fmt.Fprint(w, "}\n\n")
	}

	fmt.Fprint(w, contentBuf.String())

}

func printLooseValues(w io.Writer, valsByTypeName map[string]def.ValueRegistry) {
	// sort and refactored for cleanup/issue-3

	for k, vr := range valsByTypeName {
		// Values will be sorted by const name for extension names/spec versions, and by value for typed consts
		allValues := make([]def.ValueDefiner, 0, len(vr))
		for _, val := range vr {
			allValues = append(allValues, val)
		}

		if k == "" {
			fmt.Fprint(w, "// Extension names and versions\n")
			// Drop the values into a slice and sort by the const name
			sort.Sort(def.ByValuePublicName(allValues))
		} else {
			fmt.Fprintf(w, "// Platform-specific values for %s\n", k)
			sort.Sort(def.ByValue(allValues))
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
			var data, err1 = ioutil.ReadFile(filepath.Join(source, relPath))
			if err1 != nil {
				return err1
			}
			return ioutil.WriteFile(filepath.Join(outDirName, relPath), data, 0666)
		}
	})

	if err != nil {
		panic(err)
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

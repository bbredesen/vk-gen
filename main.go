package main

import (
	"flag"
)

var (
	inFileName, outDirName string
)

func init() {
	flag.StringVar(&inFileName, "in", "vk.xml", "File to use as the Vulkan registry")
	flag.StringVar(&outDirName, "out", "output/", "Where to save the generate source files")
}

func main() {

}

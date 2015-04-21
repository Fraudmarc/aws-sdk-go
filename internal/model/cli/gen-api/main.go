// Command aws-gen-gocli parses a JSON description of an AWS API and generates a
// Go file containing a client for the API.
//
//     aws-gen-gocli apis/s3/2006-03-03.normal.json
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"github.com/awslabs/aws-sdk-go/internal/model/api"
)

type generateInfo struct {
	*api.API
	PackageDir string
}

// newGenerateInfo initializes the service API's folder structure for a specific service.
// If the SERVICES environment variable is set, and this service is not apart of the list
// this service will be skipped.
func newGenerateInfo(modelFile, svcPath string) *generateInfo {
	g := &generateInfo{API: &api.API{}}
	g.API.Attach(modelFile)

	if svc := os.Getenv("SERVICES"); svc != "" {
		svcs := strings.Split(svc, ",")
		for _, s := range svcs {
			if s != g.API.PackageName() { // skip this non-included service
				return nil
			}
		}
	}

	// ensure the directory exists
	pkgDir := filepath.Join(svcPath, g.API.PackageName())
	os.MkdirAll(pkgDir, 0775)
	os.MkdirAll(filepath.Join(pkgDir, g.API.InterfacePackageName()), 0775)

	g.PackageDir = pkgDir

	return g
}

// Generates service api, examples, and interface from api json definition files.
//
// Flags:
// -path alternative service path to write generated files to for each service.
//
// Env:
//  SERVICES comma separated list of services to generate.
func main() {
	var svcPath string
	flag.StringVar(&svcPath, "path", "service", "generate in a specific directory (default: 'service')")
	flag.Parse()

	files := []string{}
	for i := 0; i < flag.NArg(); i++ {
		file := flag.Arg(i)
		if strings.Contains(file, "*") {
			paths, _ := filepath.Glob(file)
			files = append(files, paths...)
		} else {
			files = append(files, file)
		}
	}

	sort.Strings(files)

	// Remove old API versions from list
	m := map[string]bool{}
	for i := range files {
		idx := len(files) - 1 - i
		parts := strings.Split(files[idx], string(filepath.Separator))
		svc := parts[len(parts)-2] // service name is 2nd-to-last component

		if m[svc] {
			files[idx] = "" // wipe this one out if we already saw the service
		}
		m[svc] = true
	}

	w := sync.WaitGroup{}
	for i := range files {
		file := files[i]
		if file == "" { // empty file
			continue
		}

		w.Add(1)
		go func() {
			defer func() {
				w.Done()
				if r := recover(); r != nil {
					fmtStr := "Error generating %s\n%s\n%s\n"
					fmt.Fprintf(os.Stderr, fmtStr, file, r, debug.Stack())
				}
			}()

			if g := newGenerateInfo(file, svcPath); g != nil {
				switch g.API.PackageName() {
				case "simpledb", "importexport":
					// These services are not yet supported, do nothing.
				default:
					fmt.Printf("Generating %s (%s)...\n",
						g.API.PackageName(), g.API.Metadata.APIVersion)

					// write api.go and service.go files
					g.writeAPIFile()
					g.writeExamplesFile()
					g.writeServiceFile()
					g.writeInterfaceFile()
				}
			}
		}()
	}
	w.Wait()
}

// writeExamplesFile writes out the service example file.
func (g *generateInfo) writeExamplesFile() {
	file := filepath.Join(g.PackageDir, "examples_test.go")
	ioutil.WriteFile(file, []byte("package "+g.API.PackageName()+"_test\n\n"+g.API.ExampleGoCode()), 0664)
}

// writeServiceFile writes out the service initialization file.
func (g *generateInfo) writeServiceFile() {
	file := filepath.Join(g.PackageDir, "service.go")
	ioutil.WriteFile(file, []byte("package "+g.API.PackageName()+"\n\n"+g.API.ServiceGoCode()), 0664)
}

// writeInterfaceFile writes out the service interface file.
func (g *generateInfo) writeInterfaceFile() {
	file := filepath.Join(g.PackageDir, g.API.InterfacePackageName(), "interface.go")
	ioutil.WriteFile(file, []byte("package "+g.API.InterfacePackageName()+"\n\n"+g.API.InterfaceGoCode()), 0664)
}

const note = "// THIS FILE IS AUTOMATICALLY GENERATED. DO NOT EDIT."

// writeAPIFile writes out the service api file.
func (g *generateInfo) writeAPIFile() {
	file := filepath.Join(g.PackageDir, "api.go")
	pkg := g.API.PackageName()
	code := fmt.Sprintf("// THIS FILE IS AUTOMATICALLY GENERATED. DO NOT EDIT.\n\n"+
		"// Package %s provides a client for %s.\n"+
		"package %s\n\n%s", pkg, g.API.Metadata.ServiceFullName, pkg, g.API.APIGoCode())
	ioutil.WriteFile(file, []byte(code), 0664)
}

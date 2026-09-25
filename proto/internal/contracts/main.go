// Command contracts compares each contract's two statements of its version: the integer its path carries,
// `yoke/<contract>/v<n>/`, and the integer it states inside, the value of its enum `Contract`'s
// `CONTRACT_VERSION`. They are one number, and a contract whose two differ is malformed and is rejected
// where the definitions are built, never where two parties meet.
//
// The rule is at https://docs.yoke-project.dev/reference/plugin-surface/the-wire/#the-version.
//
// Usage: contracts <root>
package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bufbuild/protocompile"
)

// A contract's package: `yoke.<contract>.v<n>`.
var contractPackage = regexp.MustCompile(`^yoke\.([a-z][a-z0-9_]*)\.v([0-9]+)$`)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: contracts <root>")
		os.Exit(2)
	}
	problems, found, err := check(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "contracts:", err)
		os.Exit(1)
	}
	for _, problem := range problems {
		fmt.Println("contracts:", problem)
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
	fmt.Printf("contracts: %s\n", strings.Join(found, ", "))
}

// check returns what disagrees, and each contract that agrees as `<path> states <n>`.
func check(root string) (problems, found []string, err error) {
	var sources []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// A checker's fixtures are not definitions, unless they are what the checker was pointed at.
		if d.IsDir() && d.Name() == "testdata" && p != root {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".proto") {
			relative, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			sources = append(sources, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(sources) == 0 {
		return nil, nil, fmt.Errorf("no definitions under %s", root)
	}

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{ImportPaths: []string{root}}),
	}
	files, err := compiler.Compile(context.Background(), sources...)
	if err != nil {
		return nil, nil, err
	}

	stated := map[string]int{}    // a contract's directory -> the integer it states
	hasFiles := map[string]bool{} // every contract directory seen
	for _, file := range files {
		matched := contractPackage.FindStringSubmatch(string(file.Package()))
		if matched == nil {
			continue
		}
		directory := path.Dir(file.Path())
		want := "yoke/" + matched[1] + "/v" + matched[2]
		if directory != want {
			problems = append(problems, fmt.Sprintf("%s declares the package %s, which lives under %s", file.Path(), file.Package(), want))
			continue
		}
		hasFiles[directory] = true
		if contract := file.Enums().ByName("Contract"); contract != nil {
			if value := contract.Values().ByName("CONTRACT_VERSION"); value != nil {
				stated[directory] = int(value.Number())
			}
		}
	}

	directories := make([]string, 0, len(hasFiles))
	for directory := range hasFiles {
		directories = append(directories, directory)
	}
	sort.Strings(directories)
	for _, directory := range directories {
		inPath, _ := strconv.Atoi(directory[strings.LastIndex(directory, "/v")+2:])
		number, ok := stated[directory]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s states no version: it needs an enum Contract with CONTRACT_VERSION = %d", directory, inPath))
		case number != inPath:
			problems = append(problems, fmt.Sprintf("%s states %d and its path says %d — the two are one number", directory, number, inPath))
		default:
			found = append(found, fmt.Sprintf("%s states %d", directory, number))
		}
	}
	return problems, found, nil
}

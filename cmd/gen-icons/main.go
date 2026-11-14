// Command gen-icons generates the bundled icon packages icons/generic,
// icons/network and icons/brands from SVGs vendored under sources/.
//
// It lives in its own module so nothing it needs reaches users' go.mod.
// Run it from this directory:
//
//	go run .                  # regenerate every package
//	go run . -set generic     # regenerate one package (what go:generate runs)
//
// To move to new upstream versions, download and extract the npm packages
// lucide-static and simple-icons (for example
// https://registry.npmjs.org/lucide-static/-/lucide-static-1.72.0.tgz and
// https://registry.npmjs.org/simple-icons/-/simple-icons-16.37.0.tgz, the
// versions recorded in sources/*/VERSION), then vendor the icons named in
// sets.go:
//
//	go run . -import-lucide /path/to/lucide-static/package \
//	         -import-simple-icons /path/to/simple-icons/package
//	go run .
//
// Import copies only the SVGs listed in sets.go, the licence files and, for
// Simple Icons, the data entries of those icons.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

func main() {
	var (
		root        = flag.String("root", "../..", "root of the diagram module")
		only        = flag.String("set", "", "generate only this package (generic, network or brands)")
		importLuc   = flag.String("import-lucide", "", "vendor icons from an extracted lucide-static npm package directory")
		importSimpl = flag.String("import-simple-icons", "", "vendor icons from an extracted simple-icons npm package directory")
	)
	flag.Parse()
	if err := run(*root, *only, *importLuc, *importSimpl); err != nil {
		fmt.Fprintln(os.Stderr, "gen-icons:", err)
		os.Exit(1)
	}
}

const sourcesDir = "sources"

func run(root, only, importLucide, importSimple string) error {
	if importLucide != "" || importSimple != "" {
		if importLucide != "" {
			if err := importLucidePkg(importLucide, sourcesDir); err != nil {
				return fmt.Errorf("import lucide: %w", err)
			}
		}
		if importSimple != "" {
			if err := importSimpleIconsPkg(importSimple, sourcesDir); err != nil {
				return fmt.Errorf("import simple-icons: %w", err)
			}
		}
		return nil
	}
	if only != "" && !slices.ContainsFunc(sets, func(s set) bool { return s.Pkg == only }) {
		return fmt.Errorf("unknown set %q", only)
	}
	for _, s := range sets {
		if only != "" && s.Pkg != only {
			continue
		}
		out, err := generate(s, sourcesDir)
		if err != nil {
			return fmt.Errorf("%s: %w", s.Pkg, err)
		}
		if err := writeSet(filepath.Join(root, "icons", s.Pkg), out); err != nil {
			return fmt.Errorf("%s: %w", s.Pkg, err)
		}
	}
	return nil
}

// writeSet writes a package's generated files and removes generated licence
// files it no longer produces.
func writeSet(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stale, err := filepath.Glob(filepath.Join(dir, "LICENSE-*"))
	if err != nil {
		return err
	}
	for _, p := range stale {
		if _, ok := files[filepath.Base(p)]; !ok {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	for _, name := range sortedKeys(files) {
		if err := os.WriteFile(filepath.Join(dir, name), files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

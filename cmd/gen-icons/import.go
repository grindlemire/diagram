package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// slugsFor returns the sorted, de-duplicated slugs that sets.go uses from a
// source.
func slugsFor(source string) []string {
	var slugs []string
	for _, s := range sets {
		if s.Source != source {
			continue
		}
		for _, ic := range s.Icons {
			slugs = append(slugs, ic.Slug)
		}
	}
	slices.Sort(slugs)
	return slices.Compact(slugs)
}

func importLucidePkg(pkg, sources string) error {
	dst := filepath.Join(sources, "lucide")
	if err := vendorCommon(pkg, dst, slugsFor("lucide"), "LICENSE"); err != nil {
		return err
	}
	return nil
}

func importSimpleIconsPkg(pkg, sources string) error {
	dst := filepath.Join(sources, "simple-icons")
	slugs := slugsFor("simple-icons")
	if err := vendorCommon(pkg, dst, slugs, "LICENSE.md", "DISCLAIMER.md"); err != nil {
		return err
	}

	// Keep only the vendored icons' entries, unchanged, one per line.
	raw, err := os.ReadFile(filepath.Join(pkg, "data", "simple-icons.json"))
	if err != nil {
		return err
	}
	var all []json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return fmt.Errorf("simple-icons.json: %w", err)
	}
	picked := map[string][]byte{}
	for _, m := range all {
		var e siEntry
		if err := json.Unmarshal(m, &e); err != nil {
			return fmt.Errorf("simple-icons.json: %w", err)
		}
		if slices.Contains(slugs, e.Slug) {
			var c bytes.Buffer
			if err := json.Compact(&c, m); err != nil {
				return err
			}
			picked[e.Slug] = c.Bytes()
			if l := e.licenceOf(); l != "" {
				if _, err := os.Stat(filepath.Join(dst, "licenses", l+".txt")); err != nil {
					fmt.Fprintf(os.Stderr, "gen-icons: %s is %s licensed; vendor its text as %s\n",
						e.Slug, l, filepath.Join(dst, "licenses", l+".txt"))
				}
			}
		}
	}
	var out bytes.Buffer
	out.WriteString("[\n")
	for i, slug := range slugs {
		m, ok := picked[slug]
		if !ok {
			return fmt.Errorf("simple-icons.json has no entry for %q", slug)
		}
		out.Write(m)
		if i < len(slugs)-1 {
			out.WriteByte(',')
		}
		out.WriteByte('\n')
	}
	out.WriteString("]\n")
	return os.WriteFile(filepath.Join(dst, "data.json"), out.Bytes(), 0o644)
}

// vendorCommon copies the listed SVGs and files from an extracted npm package
// into dst, records the package version and removes SVGs no longer listed.
func vendorCommon(pkg, dst string, slugs []string, files ...string) error {
	var meta struct {
		Version string `json:"version"`
	}
	raw, err := os.ReadFile(filepath.Join(pkg, "package.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("package.json: %w", err)
	}
	if meta.Version == "" || strings.ContainsAny(meta.Version, " \n/") {
		return fmt.Errorf("package.json: bad version %q", meta.Version)
	}

	iconDir := filepath.Join(dst, "icons")
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return err
	}
	old, err := filepath.Glob(filepath.Join(iconDir, "*.svg"))
	if err != nil {
		return err
	}
	for _, p := range old {
		if !slices.Contains(slugs, strings.TrimSuffix(filepath.Base(p), ".svg")) {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	for _, slug := range slugs {
		if err := copyFile(filepath.Join(pkg, "icons", slug+".svg"), filepath.Join(iconDir, slug+".svg")); err != nil {
			return err
		}
	}
	for _, f := range files {
		if err := copyFile(filepath.Join(pkg, f), filepath.Join(dst, f)); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dst, "VERSION"), []byte(meta.Version+"\n"), 0o644)
}

func copyFile(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New(src + " is not a regular file")
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

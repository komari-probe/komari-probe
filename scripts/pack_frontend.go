package main

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

// The embedded default frontend combines two independently built projects:
//   - theme-nova: the default theme, tarred at the archive root (index.html, assets/, ...)
//   - admin:      the built-in admin console, tarred under "admin/" (admin.html, assets/, ...)
//
// They used to be two build outputs (index.html / admin.html) of one "web"
// monorepo sharing a single dist/ directory. Now that admin-ui lives in its
// own repository, this script merges the two dist directories itself.
func main() {
	themeDistDir := filepath.Join("..", "theme-nova", "dist")
	adminDistDir := filepath.Join("..", "admin", "dist")
	outputFile := filepath.Join("internal", "platform", "frontend", "defaultTheme", "dist.tar.zst")

	if _, err := os.Stat(themeDistDir); err != nil {
		fmt.Fprintf(os.Stderr, "theme dist directory not found at %s: %v\n", themeDistDir, err)
		os.Exit(1)
	}
	if _, err := os.Stat(adminDistDir); err != nil {
		fmt.Fprintf(os.Stderr, "admin dist directory not found at %s: %v\n", adminDistDir, err)
		os.Exit(1)
	}

	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)

	if err := addDistDir(tarWriter, themeDistDir, "", nil); err != nil {
		fmt.Fprintf(os.Stderr, "failed to tar theme dist: %v\n", err)
		os.Exit(1)
	}

	// admin.html is the fixed entry name the server looks up
	// (frontend.AdminDir / frontend.AdminIndexFile); the admin project's own
	// build output is a conventional index.html, so it is renamed on the way in.
	rename := map[string]string{"index.html": "admin.html"}
	if err := addDistDir(tarWriter, adminDistDir, "admin", rename); err != nil {
		fmt.Fprintf(os.Stderr, "failed to tar admin dist: %v\n", err)
		os.Exit(1)
	}

	if err := tarWriter.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to close tar: %v\n", err)
		os.Exit(1)
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init zstd: %v\n", err)
		os.Exit(1)
	}
	defer encoder.Close()

	compressed := encoder.EncodeAll(tarBuf.Bytes(), nil)

	if err := os.WriteFile(outputFile, compressed, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
		os.Exit(1)
	}

	// Copy theme json (both sonar-theme.json and komari-theme.json for full compatibility)
	themeJsonSrc := filepath.Join("..", "theme-nova", "komari-theme.json")
	themeData, err := os.ReadFile(themeJsonSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read theme json: %v\n", err)
		os.Exit(1)
	}

	_ = os.WriteFile(filepath.Join("internal", "platform", "frontend", "defaultTheme", "sonar-theme.json"), themeData, 0644)
	_ = os.WriteFile(filepath.Join("internal", "platform", "frontend", "defaultTheme", "komari-theme.json"), themeData, 0644)

	previewSrc := filepath.Join("..", "theme-nova", "docs", "preview.png")
	previewData, err := os.ReadFile(previewSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read theme preview image: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join("internal", "platform", "frontend", "defaultTheme", "preview.png"), previewData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write theme preview image: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully packed frontend into %s (%d bytes)\n", outputFile, len(compressed))
}

// addDistDir walks distDir and writes each entry into the tar under
// prefix/<relative path>, applying rename to any top-level file name that
// matches a key in rename.
func addDistDir(tarWriter *tar.Writer, distDir string, prefix string, rename map[string]string) error {
	return filepath.Walk(distDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(distDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		if newName, ok := rename[relPath]; ok {
			relPath = newName
		}
		if prefix != "" {
			relPath = prefix + "/" + relPath
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tarWriter, f); err != nil {
				return err
			}
		}
		return nil
	})
}

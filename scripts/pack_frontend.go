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

func main() {
	distDir := filepath.Join("..", "web", "dist")
	outputFile := filepath.Join("internal", "platform", "frontend", "defaultTheme", "dist.tar.zst")

	if _, err := os.Stat(distDir); err != nil {
		fmt.Fprintf(os.Stderr, "dist directory not found at %s: %v\n", distDir, err)
		os.Exit(1)
	}

	var tarBuf bytes.Buffer
	tarWriter := tar.NewWriter(&tarBuf)

	err := filepath.Walk(distDir, func(path string, info os.FileInfo, err error) error {
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
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to tar dist: %v\n", err)
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
	themeJsonSrcSonar := filepath.Join("..", "web", "sonar-theme.json")
	themeJsonSrcKomari := filepath.Join("..", "web", "komari-theme.json")
	var themeData []byte
	if data, err := os.ReadFile(themeJsonSrcSonar); err == nil {
		themeData = data
	} else if data, err := os.ReadFile(themeJsonSrcKomari); err == nil {
		themeData = data
	} else {
		fmt.Fprintf(os.Stderr, "failed to read theme json: %v\n", err)
		os.Exit(1)
	}

	_ = os.WriteFile(filepath.Join("internal", "platform", "frontend", "defaultTheme", "sonar-theme.json"), themeData, 0644)
	_ = os.WriteFile(filepath.Join("internal", "platform", "frontend", "defaultTheme", "komari-theme.json"), themeData, 0644)

	fmt.Printf("Successfully packed frontend into %s (%d bytes)\n", outputFile, len(compressed))
}

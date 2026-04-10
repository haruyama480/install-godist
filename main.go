package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type GoRelease struct {
	Version string   `json:"version"`
	Stable  bool     `json:"stable"`
	Files   []GoFile `json:"files"`
}

type GoFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Kind     string `json:"kind"`
	Sha256   string `json:"sha256"`
	Size     int64  `json:"size"`
}

const goDLAPI = "https://go.dev/dl/?mode=json&include=all"

func main() {
	versionFlag := flag.String("version", "latest", "Go version to install (e.g., latest, 1.22, 1.22.1)")
	destFlag := flag.String("dest", "./", "Extraction destination directory (e.g., /usr/local or ./)")
	osFlag := flag.String("os", runtime.GOOS, "Target OS (e.g., linux, darwin, windows)")
	archFlag := flag.String("arch", runtime.GOARCH, "Target Architecture (e.g., amd64, arm64)")
	unstableFlag := flag.Bool("unstable", false, "Include non-stable releases (rc, beta)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	targetVersion := *versionFlag
	installPath := *destFlag
	targetOS := *osFlag
	targetArch := *archFlag
	includeUnstable := *unstableFlag

	fmt.Println("Fetching Go release list...")
	releases, err := fetchReleases()
	if err != nil {
		fatalf("Failed to fetch releases: %v", err)
	}

	// Filter releases based on stability
	var filteredReleases []GoRelease
	for _, r := range releases {
		if !includeUnstable {
			if r.Stable == false {
				continue
			}
		}
		filteredReleases = append(filteredReleases, r)
	}

	if len(filteredReleases) == 0 {
		fatalf("No releases found matching the criteria.")
	}

	// filteredReleases is already sorted by version in descending order due to the API response, so we can skip sorting here.

	// Resolve the given version to a specific release
	selectedRelease := resolveVersion(targetVersion, filteredReleases)
	if selectedRelease == nil {
		fatalf("Error: Version '%s' not found or not available.", targetVersion)
	}

	// Find the archive file that matches the specified OS and Arch
	var selectedFile *GoFile
	for _, f := range selectedRelease.Files {
		if f.OS == targetOS && f.Arch == targetArch && f.Kind == "archive" {
			selectedFile = &f
			break
		}
	}

	if selectedFile == nil {
		fatalf("Error: No archive found for OS '%s' and Arch '%s' on version %s", targetOS, targetArch, selectedRelease.Version)
	}

	// Download
	fmt.Printf("Resolved Version: %s\n", selectedRelease.Version)
	fmt.Printf("Target Platform : %s/%s\n", targetOS, targetArch)
	fmt.Printf("Downloading %s...\n", selectedFile.Filename)

	tmpFile, err := os.CreateTemp("", "go-dl-*")
	if err != nil {
		fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if err := downloadFile("https://go.dev/dl/"+selectedFile.Filename, tmpFile); err != nil {
		fatalf("Download failed: %v", err)
	}
	tmpFile.Close()

	// Verify
	fmt.Println("Verifying file size and checksum...")
	if err := verifyFile(tmpFile.Name(), selectedFile.Size, selectedFile.Sha256); err != nil {
		fatalf("Verification failed: %v", err)
	}
	fmt.Println("Verification passed.")

	// Extract
	fmt.Printf("Extracting to %s...\n", installPath)
	if err := extractArchive(tmpFile.Name(), installPath, selectedFile.Filename); err != nil {
		fatalf("Extraction failed: %v", err)
	}

	fmt.Println("Installation completed successfully!")
}

// fetchReleases は公式APIからすべてのGoリリース情報を取得します
func fetchReleases() ([]GoRelease, error) {
	resp, err := http.Get(goDLAPI)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	var releases []GoRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	return releases, nil
}

func resolveVersion(target string, releases []GoRelease) *GoRelease {
	if target == "latest" {
		return &releases[0] // first one is the latest
	}

	targetPrefix := "go" + target
	isMinorOnly := strings.Count(target, ".") == 1

	for _, r := range releases {
		if isMinorOnly {
			if strings.HasPrefix(r.Version, targetPrefix+".") || r.Version == targetPrefix {
				return &r
			}
		} else {
			if r.Version == targetPrefix {
				return &r
			}
		}
	}
	return nil
}

func downloadFile(url string, outFile *os.File) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(outFile, resp.Body)
	return err
}

// verifyFile はファイルサイズとSHA256ハッシュが期待値と一致するか確認します
func verifyFile(filePath string, expectedSize int64, expectedSha256 string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open downloaded file: %w", err)
	}
	defer f.Close()

	// サイズの検証
	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stats: %w", err)
	}
	if stat.Size() != expectedSize {
		return fmt.Errorf("size mismatch: expected %d bytes, got %d bytes", expectedSize, stat.Size())
	}

	// SHA256ハッシュの検証
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to read file for hashing: %w", err)
	}

	calculatedSha256 := hex.EncodeToString(hasher.Sum(nil))
	if calculatedSha256 != expectedSha256 {
		return fmt.Errorf("sha256 mismatch: expected %s, got %s", expectedSha256, calculatedSha256)
	}

	return nil
}

// extractArchive は拡張子に応じて展開処理を振り分けます
func extractArchive(archivePath, destDir, filename string) error {
	if strings.HasSuffix(filename, ".tar.gz") {
		return extractTarGz(archivePath, destDir)
	} else if strings.HasSuffix(filename, ".zip") {
		return extractZip(archivePath, destDir)
	}
	return fmt.Errorf("unsupported archive format: %s", filename)
}

// extractTarGz は .tar.gz ファイルを展開します
func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target, err := sanitizeExtractPath(destDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

// extractZip は .zip ファイルを展開します
func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target, err := sanitizeExtractPath(destDir, f.Name)
		if err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, f.Mode())
			continue
		}

		os.MkdirAll(filepath.Dir(target), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// sanitizeExtractPath はZipSlip脆弱性を防ぐためのパス検証を行います
func sanitizeExtractPath(dest, filePath string) (string, error) {
	destPath := filepath.Clean(dest)
	targetPath := filepath.Join(destPath, filePath)
	if !strings.HasPrefix(targetPath, destPath+string(os.PathSeparator)) && targetPath != destPath {
		return "", fmt.Errorf("illegal file path: %s", filePath)
	}
	return targetPath, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

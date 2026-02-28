package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testRestore simulates the restore logic from RestoreCmd to test tar entry handling.
func testRestore(tarGzData []byte, projectRoot string) error {
	gr, err := gzip.NewReader(bytes.NewReader(tarGzData))
	if err != nil {
		return fmt.Errorf("reading archive: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive entry: %w", err)
		}

		// Prevent path traversal
		target := filepath.Join(projectRoot, header.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(projectRoot)+string(os.PathSeparator)) {
			return fmt.Errorf("archive contains invalid path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return fmt.Errorf("creating parent directory: %w", err)
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err != nil {
				return fmt.Errorf("creating file: %w", err)
			}
			if _, err := io.Copy(outFile, io.LimitReader(tr, 100*1024*1024)); err != nil {
				outFile.Close()
				return fmt.Errorf("writing file: %w", err)
			}
			outFile.Close()
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive contains unsafe entry type (symlink/hardlink): %s", header.Name)
		default:
			return fmt.Errorf("archive contains unsupported entry type %d: %s", header.Typeflag, header.Name)
		}
	}
	return nil
}

func createTarGz(t *testing.T, entries []tar.Header, contents map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, h := range entries {
		content := contents[h.Name]
		h.Size = int64(len(content))
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("WriteHeader(%s): %v", h.Name, err)
		}
		if content != "" {
			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatalf("Write(%s): %v", h.Name, err)
			}
		}
	}

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func TestBackupRestoreRejectsSymlinks(t *testing.T) {
	projectRoot := t.TempDir()

	// Create archive with a symlink entry
	archive := createTarGz(t, []tar.Header{
		{Name: ".humsafe/", Typeflag: tar.TypeDir, Mode: 0700},
		{Name: ".humsafe/evil-link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
	}, nil)

	err := testRestore(archive, projectRoot)
	if err == nil {
		t.Fatal("expected error for archive with symlink, got nil")
	}
	if !strings.Contains(err.Error(), "symlink/hardlink") {
		t.Errorf("expected symlink error, got: %v", err)
	}
}

func TestBackupRestoreRejectsHardlinks(t *testing.T) {
	projectRoot := t.TempDir()

	// Create archive with a hardlink entry
	archive := createTarGz(t, []tar.Header{
		{Name: ".humsafe/", Typeflag: tar.TypeDir, Mode: 0700},
		{Name: ".humsafe/hard-link", Typeflag: tar.TypeLink, Linkname: "/etc/shadow"},
	}, nil)

	err := testRestore(archive, projectRoot)
	if err == nil {
		t.Fatal("expected error for archive with hardlink, got nil")
	}
	if !strings.Contains(err.Error(), "symlink/hardlink") {
		t.Errorf("expected hardlink error, got: %v", err)
	}
}

func TestBackupRestoreAcceptsNormalFiles(t *testing.T) {
	projectRoot := t.TempDir()

	archive := createTarGz(t, []tar.Header{
		{Name: ".humsafe/", Typeflag: tar.TypeDir, Mode: 0700},
		{Name: ".humsafe/config.json", Typeflag: tar.TypeReg, Mode: 0600},
	}, map[string]string{
		".humsafe/config.json": `{"version": 1}`,
	})

	err := testRestore(archive, projectRoot)
	if err != nil {
		t.Fatalf("expected no error for normal archive, got: %v", err)
	}

	// Verify file was created
	content, err := os.ReadFile(filepath.Join(projectRoot, ".humsafe", "config.json"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(content) != `{"version": 1}` {
		t.Errorf("unexpected content: %s", content)
	}
}

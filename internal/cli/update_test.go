package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"testing"
)

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	return buf.Bytes()
}

func TestExtractArchive(t *testing.T) {
	want := []byte("fake openexec binary bytes")

	cases := []struct {
		name    string
		archive string
		data    []byte
	}{
		{"tar.gz plain", "openexec-linux-amd64.tar.gz", makeTarGz(t, "openexec", want)},
		{"tar.gz with dir prefix", "openexec-linux-amd64.tar.gz", makeTarGz(t, "dist/openexec", want)},
		{"zip windows", "openexec-windows-amd64.zip", makeZip(t, "openexec.exe", want)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := extractArchive(tc.archive, tc.data)
			if err != nil {
				t.Fatalf("extractArchive: %v", err)
			}
			defer os.Remove(p)
			got, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("extracted bytes mismatch: got %q want %q", got, want)
			}
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm()&0100 == 0 {
				t.Fatalf("extracted file not executable: %v", fi.Mode())
			}
		})
	}
}

func TestExtractArchive_MissingBinary(t *testing.T) {
	data := makeTarGz(t, "README.md", []byte("not a binary"))
	if _, err := extractArchive("openexec-linux-amd64.tar.gz", data); err == nil {
		t.Fatal("expected error when archive has no openexec binary")
	}
}

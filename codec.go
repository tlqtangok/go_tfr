package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const GZIP_THRESHOLD = 2048 // bytes

// buildMetaHeader builds the metadata header prepended to Redis values.
// Format (byte-identical to Perl):
//   GZIP_:0\n  (or GZIP_:1)
//   FILENAME_:filename\n
//   CRC32_:crc32value\n
//   <payload bytes>
func buildMetaHeader(isGzip bool, filename string, crc uint32) []byte {
	gz := "0"
	if isGzip {
		gz = "1"
	}
	header := fmt.Sprintf("GZIP_:%s\nFILENAME_:%s\nCRC32_:%d\n", gz, filename, crc)
	return []byte(header)
}

// parseMetaHeader splits raw Redis value into (isGzip, filename, crc32, payload).
func parseMetaHeader(raw []byte) (isGzip bool, filename string, crc32val uint32, payload []byte, err error) {
	// Read header lines until we've consumed GZIP_, FILENAME_, CRC32_
	reader := bytes.NewReader(raw)
	br := io.Reader(reader)

	readLine := func() (string, error) {
		var buf []byte
		b := make([]byte, 1)
		for {
			_, e := br.Read(b)
			if e != nil {
				return string(buf), e
			}
			if b[0] == '\n' {
				return string(buf), nil
			}
			buf = append(buf, b[0])
		}
	}

	// Line 1: GZIP_:0or1
	line1, e := readLine()
	if e != nil {
		err = fmt.Errorf("header line1: %v", e)
		return
	}
	line1 = strings.TrimPrefix(line1, "GZIP_:")
	isGzip = (line1 == "1")

	// Line 2: FILENAME_:name
	line2, e := readLine()
	if e != nil {
		err = fmt.Errorf("header line2: %v", e)
		return
	}
	filename = strings.TrimPrefix(line2, "FILENAME_:")

	// Line 3: CRC32_:value
	line3, e := readLine()
	if e != nil {
		err = fmt.Errorf("header line3: %v", e)
		return
	}
	crcStr := strings.TrimPrefix(line3, "CRC32_:")
	var cv uint64
	_, scanErr := fmt.Sscanf(crcStr, "%d", &cv)
	if scanErr != nil {
		err = fmt.Errorf("bad crc32: %v", scanErr)
		return
	}
	crc32val = uint32(cv)

	// Rest is payload
	pos, _ := reader.Seek(0, io.SeekCurrent)
	payload = raw[pos:]
	return
}

// gzipBytes compresses data.
func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gunzipBytes decompresses data.
func gunzipBytes(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// tarFolder creates a .tar.gz of a directory, returns bytes.
func tarFolder(dir string) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	base := filepath.Base(dir)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filepath.Dir(dir), path)
		rel = filepath.ToSlash(rel)

		hdr, e := tar.FileInfoHeader(info, "")
		if e != nil {
			return e
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.IsDir() {
			f, e := os.Open(path)
			if e != nil {
				return e
			}
			defer f.Close()
			_, e = io.Copy(tw, f)
			return e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = base
	tw.Close()
	gz.Close()
	return buf.Bytes(), nil
}

// untarBytes extracts a tar.gz to destDir.
func untarBytes(data []byte, destDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		if hdr.FileInfo().IsDir() {
			os.MkdirAll(target, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(target), 0755)
		f, err := os.Create(target)
		if err != nil {
			return err
		}
		io.Copy(f, tr)
		f.Close()
	}
	return nil
}

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
	"sync"
)

const GZIP_THRESHOLD = 2048

var gzipWriterPool = sync.Pool{
	New: func() interface{} {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return w
	},
}

var bufPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// buildMetaHeader builds the 3-line metadata header (byte-identical to Perl).
func buildMetaHeader(isGzip bool, filename string, crc uint32) []byte {
	gz := "0"
	if isGzip {
		gz = "1"
	}
	return []byte(fmt.Sprintf("GZIP_:%s\nFILENAME_:%s\nCRC32_:%d\n", gz, filename, crc))
}

// parseMetaHeader splits raw Redis value into (isGzip, filename, crc32, payload).
func parseMetaHeader(raw []byte) (isGzip bool, filename string, crc32val uint32, payload []byte, err error) {
	r := bytes.NewReader(raw)

	readLine := func() (string, error) {
		var buf []byte
		b := [1]byte{}
		for {
			_, e := r.Read(b[:])
			if e != nil {
				return string(buf), e
			}
			if b[0] == '\n' {
				return string(buf), nil
			}
			buf = append(buf, b[0])
		}
	}

	line1, e := readLine()
	if e != nil {
		err = fmt.Errorf("header line1: %v", e)
		return
	}
	if idx := strings.Index(line1, ":"); idx >= 0 {
		isGzip = line1[idx+1:] == "1"
	}

	line2, e := readLine()
	if e != nil {
		err = fmt.Errorf("header line2: %v", e)
		return
	}
	if idx := strings.Index(line2, ":"); idx >= 0 {
		filename = line2[idx+1:]
	}

	line3, e := readLine()
	if e != nil {
		err = fmt.Errorf("header line3: %v", e)
		return
	}
	crcStr := ""
	if idx := strings.Index(line3, ":"); idx >= 0 {
		crcStr = line3[idx+1:]
	}
	var cv uint64
	if _, scanErr := fmt.Sscanf(crcStr, "%d", &cv); scanErr != nil {
		err = fmt.Errorf("bad crc32: %v", scanErr)
		return
	}
	crc32val = uint32(cv)

	pos, _ := r.Seek(0, io.SeekCurrent)
	payload = raw[pos:]
	return
}

// gzipBytes compresses data at BestSpeed (3-5x faster than default).
func gzipBytes(data []byte) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	w := gzipWriterPool.Get().(*gzip.Writer)
	w.Reset(buf)
	defer gzipWriterPool.Put(w)

	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
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

// tarFolder creates a .tar.gz archive of dir in memory (matches Perl tar_folder_to_file_tgz).
func tarFolder(dir string) ([]byte, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	gz, _ := gzip.NewWriterLevel(buf, gzip.BestSpeed)
	tw := tar.NewWriter(gz)

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	parentDir := filepath.Dir(absDir)

	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(parentDir, path)
		rel = filepath.ToSlash(rel)

		hdr, e := tar.FileInfoHeader(info, "")
		if e != nil {
			return e
		}
		hdr.Name = rel
		if info.IsDir() {
			hdr.Name += "/"
		}
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
	tw.Close()
	gz.Close()

	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

// untarToDir extracts a .tar.gz archive (bytes) to destDir.
func untarToDir(data []byte, destDir string) error {
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
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
		if err != nil {
			return err
		}
		io.Copy(f, tr)
		f.Close()
	}
	return nil
}

// isTextData returns true if data looks like text (matches Perl -T operator).
// Perl's -T heuristic: any NUL byte → binary; otherwise < 10% non-printable bytes.
func isTextData(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	// Perl -T: a single NUL byte means binary
	for _, b := range check {
		if b == 0 {
			return false
		}
	}
	binary := 0
	for _, b := range check {
		if b < 7 || (b >= 14 && b < 32 && b != 27) {
			binary++
		}
	}
	return float64(binary)/float64(len(check)) < 0.10
}

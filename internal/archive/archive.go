// Package archive extracts tar and zip archives, Django's
// django.utils.archive. startproject and startapp extract a template given
// as an archive with it.
package archive

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// UnrecognizedFormatError reports a path whose extension names no archive
// format, Django's UnrecognizedArchiveFormat.
type UnrecognizedFormatError struct {
	// Path is the archive.
	Path string
}

// Error returns Django's message.
func (e *UnrecognizedFormatError) Error() string {
	return "Path not a recognized archive format: " + e.Path
}

// InvalidPathError reports an entry that would be written outside the target
// directory, the SuspiciousOperation of Django's extraction.
type InvalidPathError struct {
	// Name is the name of the entry, without the leading directory when that
	// is left out.
	Name string
}

// Error returns Django's message.
func (e *InvalidPathError) Error() string {
	return "Archive contains invalid path: '" + e.Name + "'"
}

// formats maps the extensions of Django's extension_map to the formats.
// Django reads the lzma and xz tar archives too; Go has no reader for them,
// so Extract reports their compression as unsupported.
var formats = map[string]string{
	".tar": "tar", ".tar.bz2": "tar", ".tbz2": "tar", ".tbz": "tar", ".tz2": "tar",
	".tar.gz": "tar", ".tgz": "tar", ".taz": "tar", ".tar.lzma": "tar", ".tlz": "tar",
	".tar.xz": "tar", ".txz": "tar", ".zip": "zip",
}

// Extract unpacks the tar or zip archive at path into the directory to. The
// extension of path selects the format, as in Django: a tar archive is read
// with or without gzip or bzip2 compression.
//
// When every entry lies below the same top directory, that directory is left
// out. A file keeps the permission bits of the archive when they make it
// readable for everyone; otherwise the umask decides. Only regular files and
// directories are extracted; links and other entries are skipped. An entry
// that would land outside to stops the extraction with an InvalidPathError.
func Extract(path, to string) error {
	switch format(path) {
	case "tar":
		return extractTar(path, to)
	case "zip":
		return extractZip(path, to)
	default:
		return &UnrecognizedFormatError{Path: path}
	}
}

// format returns "tar" or "zip" for the extension of path, or "": the last
// extension, then the one before it, as Django's Archive._archive_cls looks
// them up.
func format(path string) string {
	base := strings.ToLower(path)
	ext := filepath.Ext(base)
	if f, ok := formats[ext]; ok {
		return f
	}
	return formats[filepath.Ext(strings.TrimSuffix(base, ext))]
}

// extractTar extracts a tar archive. The names are read first, to find the
// leading directory, as Django's TarArchive reads all members first.
func extractTar(path, to string) error {
	var names []string
	err := walkTar(path, func(hdr *tar.Header, _ io.Reader) error {
		names = append(names, tarName(hdr))
		return nil
	})
	if err != nil {
		return err
	}
	leading := hasLeadingDir(names)
	return walkTar(path, func(hdr *tar.Header, r io.Reader) error {
		name := tarName(hdr)
		if leading {
			_, name = splitLeadingDir(name)
		}
		filename, err := targetFilename(to, name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			return os.MkdirAll(filename, 0o777)
		case tar.TypeReg:
			return writeFile(filename, r, hdr.Mode)
		default:
			return nil
		}
	})
}

// tarName returns the name of a tar entry without the trailing slashes of a
// directory, as Python's tarfile reports it.
func tarName(hdr *tar.Header) string {
	if hdr.Typeflag == tar.TypeDir {
		return strings.TrimRight(hdr.Name, "/")
	}
	return hdr.Name
}

// walkTar calls fn with every entry of the tar archive at path, after
// undoing its compression.
func walkTar(path string, fn func(hdr *tar.Header, r io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	r, err := decompress(bufio.NewReader(f))
	if err != nil {
		return err
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		// The name is checked against the target directory below.
		case err != nil && !errors.Is(err, tar.ErrInsecurePath):
			return err
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if err := fn(hdr, tr); err != nil {
			return err
		}
	}
}

// decompress returns the reader of the tar data in r, which is plain,
// gzip-compressed or bzip2-compressed, as Python's tarfile finds out.
func decompress(r *bufio.Reader) (io.Reader, error) {
	magic, err := r.Peek(6)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(magic, []byte{0x1f, 0x8b}):
		return gzip.NewReader(r)
	case bytes.HasPrefix(magic, []byte("BZh")):
		return bzip2.NewReader(r), nil
	case bytes.HasPrefix(magic, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		return nil, errors.New("xz compression is not supported")
	case bytes.HasPrefix(magic, []byte{0x5d, 0x00, 0x00}):
		return nil, errors.New("lzma compression is not supported")
	default:
		return r, nil
	}
}

// extractZip extracts a zip archive.
func extractZip(path, to string) error {
	zr, err := zip.OpenReader(path)
	// The names are checked against the target directory below.
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return err
	}
	defer func() { _ = zr.Close() }()
	names := make([]string, len(zr.File))
	for i, f := range zr.File {
		names[i] = f.Name
	}
	leading := hasLeadingDir(names)
	for _, f := range zr.File {
		name := f.Name
		if leading {
			_, name = splitLeadingDir(name)
		}
		if name == "" {
			continue
		}
		filename, err := targetFilename(to, name)
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, "/") || strings.HasSuffix(name, `\`) {
			if err := os.MkdirAll(filename, 0o777); err != nil {
				return err
			}
			continue
		}
		if err := extractZipFile(f, filename); err != nil {
			return err
		}
	}
	return nil
}

// extractZipFile writes one file of a zip archive. The permission bits are
// the Unix mode in the upper half of the external attributes, as Django
// reads them.
func extractZipFile(f *zip.File, filename string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	return writeFile(filename, r, int64(f.ExternalAttrs>>16))
}

// writeFile writes the content of r into filename and copies the permission
// bits mode when they make the file readable for everyone, Django's
// _copy_permissions; the bits for set-user-ID and the like are dropped.
func writeFile(filename string, r io.Reader, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o777); err != nil {
		return err
	}
	out, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if mode&0o004 != 0 {
		return os.Chmod(filename, fs.FileMode(mode&0o777))
	}
	return nil
}

// splitLeadingDir splits a name into its first component and the rest at the
// first slash or backslash, after removing the leading ones.
func splitLeadingDir(name string) (dir, rest string) {
	name = strings.TrimLeft(strings.TrimLeft(name, "/"), `\`)
	slash, backslash := strings.Index(name, "/"), strings.Index(name, `\`)
	switch {
	case slash >= 0 && (backslash < 0 || slash < backslash):
		return name[:slash], name[slash+1:]
	case backslash >= 0:
		return name[:backslash], name[backslash+1:]
	default:
		return name, ""
	}
}

// hasLeadingDir reports whether all names start with the same component.
func hasLeadingDir(names []string) bool {
	var common string
	for i, name := range names {
		prefix, _ := splitLeadingDir(name)
		switch {
		case prefix == "":
			return false
		case i == 0:
			common = prefix
		case prefix != common:
			return false
		}
	}
	return true
}

// targetFilename returns the path an entry named name is written to, which
// must lie in the directory to. An absolute name stays absolute, as with
// Python's os.path.join, and is refused.
func targetFilename(to, name string) (string, error) {
	target, err := filepath.Abs(to)
	if err != nil {
		return "", fmt.Errorf("archive: %w", err)
	}
	filename := filepath.Join(target, name)
	if filepath.IsAbs(name) {
		filename = filepath.Clean(name)
	}
	if filename != target && !strings.HasPrefix(filename, target+string(filepath.Separator)) {
		return "", &InvalidPathError{Name: name}
	}
	return filename, nil
}

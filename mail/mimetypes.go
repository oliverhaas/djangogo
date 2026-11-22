package mail

import (
	"mime"
	"strings"
)

// suffixAliases are the extensions that Python's mimetypes reads as two,
// as its suffix_map.
var suffixAliases = map[string]string{
	".svgz": ".svg.gz", ".tgz": ".tar.gz", ".taz": ".tar.gz", ".tz": ".tar.gz",
	".tbz2": ".tar.bz2", ".txz": ".tar.xz",
}

// compressionSuffixes are the extensions that name a compression, which
// Python's mimetypes skips to guess the type of the file inside, as its
// encodings_map. The match is case-sensitive.
var compressionSuffixes = []string{".gz", ".Z", ".bz2", ".xz", ".br"}

// GuessType returns the MIME type of a file from its name, without
// parameters, as Python's mimetypes.guess_type does, or "" when the
// extension is unknown. The type of "notes.txt.gz" is "text/plain".
func GuessType(filename string) string {
	base, ext := splitExt(filename)
	for {
		alias, ok := suffixAliases[strings.ToLower(ext)]
		if !ok {
			break
		}
		base, ext = splitExt(base + alias)
	}
	for _, s := range compressionSuffixes {
		if ext == s {
			_, ext = splitExt(base)
			break
		}
	}
	if ext == "" {
		return ""
	}
	t := mime.TypeByExtension(ext)
	if t == "" {
		return ""
	}
	media, _, err := mime.ParseMediaType(t)
	if err != nil {
		return ""
	}
	return media
}

// splitExt splits a path before the extension of its last element, as
// Python's posixpath.splitext does: dots that start the name are no
// extension.
func splitExt(path string) (string, string) {
	dot := strings.LastIndex(path, ".")
	slash := strings.LastIndex(path, "/")
	if dot <= slash {
		return path, ""
	}
	for i := slash + 1; i < dot; i++ {
		if path[i] != '.' {
			return path[:dot], path[dot:]
		}
	}
	return path, ""
}

package clipboard

import "strings"

var mimeExt = map[string]string{
	"application/pdf":              "pdf",
	"application/json":             "json",
	"application/zip":              "zip",
	"application/gzip":             "gz",
	"application/x-gzip":           "gz",
	"application/x-tar":            "tar",
	"application/x-bzip2":          "bz2",
	"application/x-7z-compressed":  "7z",
	"application/x-rar-compressed": "rar",
	"application/vnd.rar":          "rar",
	"application/msword":           "doc",
	"application/rtf":              "rtf",
	"text/rtf":                     "rtf",
	"application/xml":              "xml",
	"text/xml":                     "xml",
	"text/html":                    "html",
	"text/css":                     "css",
	"text/csv":                     "csv",
	"text/markdown":                "md",
	"text/javascript":              "js",
	"application/javascript":       "js",
	"text/plain":                   "txt",
	"audio/mpeg":                   "mp3",
	"audio/wav":                    "wav",
	"audio/x-wav":                  "wav",
	"audio/ogg":                    "ogg",
	"application/ogg":              "ogg",
	"audio/flac":                   "flac",
	"video/mp4":                    "mp4",
	"video/webm":                   "webm",
	"video/quicktime":              "mov",
	"video/x-matroska":             "mkv",
	"image/jpeg":                   "jpg",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
	"application/vnd.ms-excel": "xls",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         "xlsx",
	"application/vnd.ms-powerpoint":                                             "ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": "pptx",
}

func ext(mime string) string {
	mime = strings.TrimSpace(strings.SplitN(mime, ";", 2)[0])
	if e, ok := mimeExt[mime]; ok {
		return e
	}
	if i := strings.IndexByte(mime, '/'); i >= 0 {
		sub := mime[i+1:]
		sub = strings.TrimPrefix(sub, "x-")
		if j := strings.IndexByte(sub, '+'); j >= 0 {
			sub = sub[:j]
		}
		if sub != "" {
			return sub
		}
	}
	return "bin"
}

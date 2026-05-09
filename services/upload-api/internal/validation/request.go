package validation

var AllowedContentTypes = map[string]bool{
	"video/mp4":        true,
	"video/webm":       true,
	"vidoe/quicktime":  true,
	"video/x-msvideo":  true,
	"video/x-matroska": true,
	"video/x-flv":      true,
	"video/3gpp":       true,
	"video/MP2T":       true,
}

func IsAllowedContentType(contentType string) bool {
	return AllowedContentTypes[contentType]
}

func AllowedContentTypesList() []string {
	types := make([]string, 0, len(AllowedContentTypes))
	for ct := range AllowedContentTypes {
		types = append(types, ct)
	}
	return types
}

// Max 256 chars, Strip path separators, null bytes, control characters
func IsValidFilename(filename string) bool {
	if len(filename) == 0 || len(filename) > 256 {
		return false
	}
	for _, r := range filename {
		if r == '/' || r == '\\' || r == 0 || (r < 32 && r != '\t') {
			return false
		}
	}
	return true
}

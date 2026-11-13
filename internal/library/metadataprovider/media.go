package metadataprovider

import (
	"os"
	"path/filepath"
	"strings"
)

// videoExtensions are the containers a media file may use.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".ts": true, ".m2ts": true,
	".mov": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".webm": true,
}

// IsVideoFile reports whether name is a media file by its extension.
func IsVideoFile(name string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(name))]
}

// SoleVideo reports whether media is the only video in its folder. A sidecar named for the folder
// rather than the file (Kodi's movie.nfo, poster.jpg) describes the folder, so in a folder of
// several videos it describes none of them.
func SoleVideo(media string) bool {
	entries, err := os.ReadDir(filepath.Dir(media))
	if err != nil {
		return false
	}
	videos := 0
	for _, e := range entries {
		if !e.IsDir() && IsVideoFile(e.Name()) {
			videos++
		}
	}
	return videos <= 1
}

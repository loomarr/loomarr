package source

import (
	"path"
	"strings"
)

// The scanned providers' IDs. A source row's ID is also one of these for the two configuration-
// backed rows, which is why they are named.
const (
	FolderID  ID = "folder"
	LibraryID ID = "library"
)

// Scanned is a provider whose sources are read where they are, such as a folder or the media
// library, and never fetched. It has no Lister, so filler never checks its sources, and no
// Addresser, so none can be registered by hand.
func Scanned(id ID, label, addressHint string) Provider {
	return scanned{id: id, description: Description{Label: label, AddressHint: addressHint}}
}

// Folder is the watched-directory provider.
func Folder() Provider {
	return scanned{id: FolderID, address: folderPath, description: Description{
		Label:        "Folder",
		AddressHint:  "/data/filler — a full path Loomarr can read",
		AddressLabel: "Folder path",
		SourceNoun:   "Folder on this server",
		BadAddress: Problem{
			Title: "That doesn't look like a folder Loomarr can watch",
			Detail: "Give a full path starting with / — for example /data/filler. " +
				"A relative path would mean different directories depending on how Loomarr was started.",
		},
	}}
}

// Library is the library-folder provider: one of the household library's folders, copied from
// and never drained (#2151 decision 36).
func Library() Provider {
	return scanned{id: LibraryID, address: folderPath, description: Description{
		Label:        "Media library",
		AddressHint:  "/media/commercials — a folder in your library",
		AddressLabel: "Folder path",
		SourceNoun:   "Folder in your library",
		BadAddress: Problem{
			Title: "That doesn't look like a folder Loomarr can read",
			Detail: "Give a full path starting with / — for example /media/commercials. " +
				"Loomarr copies clips from it and never changes it.",
		},
	}}
}

type scanned struct {
	id          ID
	description Description
	address     func(string) string
}

func (s scanned) ID() ID                { return s.id }
func (s scanned) Describe() Description { return s.description }

// Address implements Addresser. A scanned provider with no address rule takes no sources by hand.
func (s scanned) Address(raw string) string {
	if s.address == nil {
		return ""
	}
	return s.address(raw)
}

// folderPath validates a watched-folder path an operator typed.
//
// It is absolute only, and that is a correctness rule rather than a style preference. A relative
// path resolves against Loomarr's working directory, which is an implementation detail the
// operator cannot see and which differs between a container and a `go run`. "ads" would silently
// mean different directories in dev and in production, and the failure, an empty catalog, says
// nothing about why.
//
// A `..` is refused even inside an absolute path. It cannot escape anything by itself here, since
// the scan is confined to whatever this resolves to, but a path that needs cleaning is a path the
// operator did not mean to type, and storing the pre-cleaned form makes the row disagree with the
// directory it names.
func folderPath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" || !strings.HasPrefix(p, "/") {
		return ""
	}
	if strings.Contains(p, "..") {
		return ""
	}
	// Cleaned so `/data/filler/` and `/data//filler` are one source rather than two rows watching
	// one directory.
	cleaned := path.Clean(p)
	if cleaned == "/" {
		// Scanning the filesystem root would walk the entire machine. Refused as an obvious
		// mis-type rather than accepted as an heroic instruction.
		return ""
	}
	return cleaned
}

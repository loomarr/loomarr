package main

import (
	"strings"
	"testing"
)

func TestLOCSearchIsBoundedToNamedCollectionAndQuery(t *testing.T) {
	raw, err := locSearchURL(defaultBaseURL, "advertising")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "/collections/national-screening-room/") || !strings.Contains(raw, "c=50") || !strings.Contains(raw, "q=advertising") {
		t.Fatalf("URL = %s", raw)
	}
}

func TestLOCItemIDRejectsArticlesAndForeignHosts(t *testing.T) {
	if got := locItemID("http://www.loc.gov/item/97516772/"); got != "97516772" {
		t.Fatalf("ID = %q", got)
	}
	for _, raw := range []string{"https://www.loc.gov/collections/national-screening-room/articles/foo/", "https://example.com/item/97516772/"} {
		if got := locItemID(raw); got != "" {
			t.Errorf("%s => %q", raw, got)
		}
	}
}

func TestSelectMP4RefusesRestrictedOversizedAndForeignMedia(t *testing.T) {
	item := itemResponse{}
	item.Resources = append(item.Resources, struct {
		DownloadRestricted bool `json:"download_restricted"`
		Files              [][]struct {
			MIMEType string `json:"mimetype"`
			URL      string `json:"url"`
			Size     int64  `json:"size"`
		} `json:"files"`
	}{Files: [][]struct {
		MIMEType string `json:"mimetype"`
		URL      string `json:"url"`
		Size     int64  `json:"size"`
	}{{
		{MIMEType: "video/mp4", URL: "https://example.com/clip.mp4", Size: 10},
		{MIMEType: "video/mp4", URL: "https://tile.loc.gov/clip.mp4", Size: 1000},
		{MIMEType: "video/mp4", URL: "https://tile.loc.gov/clip-small.mp4", Size: 10},
	}}})
	got, ok := selectMP4(item, 100, 100)
	if !ok || got.URL != "https://tile.loc.gov/clip-small.mp4" {
		t.Fatalf("media = %+v, %v", got, ok)
	}
}

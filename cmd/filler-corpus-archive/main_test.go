package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseCandidateRequiresMatchingAllowlistedItemLicense(t *testing.T) {
	raw := []byte(`{
  "metadata":{"identifier":"soda-ad","mediatype":"movies","title":"Soda advert","collection":["classic_tv_commercials"],"licenseurl":"http://creativecommons.org/publicdomain/mark/1.0/"},
  "files":[
    {"name":"master.mp4","format":"MPEG4","source":"original","size":"20000000"},
    {"name":"small.ia.mp4","format":"h.264 IA","source":"derivative","size":"4000000","sha1":"abc","length":"30.0","height":"480"}
  ]
}`)
	got, ok := parseCandidate(defaultBaseURL, "classic_tv_commercials", "soda-ad", "https://creativecommons.org/publicdomain/mark/1.0/", "soda-ad.json", time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC), raw)
	if !ok || got.File.Name != "small.ia.mp4" || got.File.Bytes != 4_000_000 || got.MetadataSHA256 == "" {
		t.Fatalf("candidate = %+v, %v", got, ok)
	}
	if _, ok := parseCandidate(defaultBaseURL, "classic_tv_commercials", "soda-ad", "https://creativecommons.org/licenses/by-nc/4.0/", "soda-ad.json", time.Time{}, raw); ok {
		t.Fatal("search/item license disagreement was accepted")
	}
}

func TestAllowedLicenseRejectsNCAndND(t *testing.T) {
	for _, license := range []string{
		"https://creativecommons.org/licenses/by-nc-sa/4.0/",
		"https://creativecommons.org/licenses/by-nd/4.0/",
		"",
	} {
		if allowedLicense(license) {
			t.Errorf("allowed %q", license)
		}
	}
	for _, license := range []string{
		"http://creativecommons.org/licenses/publicdomain/",
		"https://creativecommons.org/publicdomain/zero/1.0/",
		"https://creativecommons.org/licenses/by/4.0/",
		"https://creativecommons.org/licenses/by-sa/4.0/",
	} {
		if !allowedLicense(license) {
			t.Errorf("rejected %q", license)
		}
	}
}

func TestMetadataFieldsAcceptArchiveStringOrArrayShapes(t *testing.T) {
	if got := stringsFromRaw(json.RawMessage(`"one"`)); len(got) != 1 || got[0] != "one" {
		t.Fatalf("single = %v", got)
	}
	if got := stringsFromRaw(json.RawMessage(`["one","two"]`)); len(got) != 2 {
		t.Fatalf("many = %v", got)
	}
}

func TestRunRequiresHardCeilingsAndIdentity(t *testing.T) {
	if code := run(nil, testWriter{t}, testWriter{t}); code != 2 {
		t.Fatalf("exit = %d", code)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { return len(p), nil }

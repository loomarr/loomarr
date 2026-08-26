// Package fillercorpus owns the source-neutral, non-authorizing inventory
// contract used to qualify certification corpus lanes.
package fillercorpus

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

const PilotSchemaVersion = 1

var PilotAuthorities = []string{
	"archive.org/prelinger",
	"loc.gov/national-screening-room",
	"images.nasa.gov",
	"cdc.gov",
	"commons.wikimedia.org",
}

type Pilot struct {
	SchemaVersion int       `json:"schemaVersion"`
	SnapshotAt    time.Time `json:"snapshotAt"`
	LockedAt      time.Time `json:"lockedAt"`
	Lanes         []Lane    `json:"lanes"`
}

type Lane struct {
	Authority              string      `json:"authority"`
	MaxRequests            int         `json:"maxRequests"`
	RequestsUsed           int         `json:"requestsUsed"`
	MaxResponseBytes       int64       `json:"maxResponseBytes"`
	ResponseBytes          int64       `json:"responseBytes"`
	MaxPredictedMediaBytes int64       `json:"maxPredictedMediaBytes"`
	PredictedMediaBytes    int64       `json:"predictedMediaBytes"`
	MaxWallTimeMS          int64       `json:"maxWallTimeMs"`
	WallTimeMS             int64       `json:"wallTimeMs"`
	Cases                  []Candidate `json:"cases"`
}

type Candidate struct {
	ItemID              string         `json:"itemId"`
	Title               string         `json:"title"`
	RoleHints           []string       `json:"roleHints"`
	ItemURL             string         `json:"itemUrl"`
	MetadataURL         string         `json:"metadataUrl"`
	MetadataRetrievedAt time.Time      `json:"metadataRetrievedAt"`
	MetadataSHA256      string         `json:"metadataSha256"`
	RightsAssertions    []string       `json:"rightsAssertions"`
	LicenseURL          string         `json:"licenseUrl,omitempty"`
	Representation      Representation `json:"representation"`
}

type Representation struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	MIMEType string `json:"mimeType"`
	Bytes    int64  `json:"bytes"`
	SHA1     string `json:"sha1,omitempty"`
	MD5      string `json:"md5,omitempty"`
}

func ValidatePilot(p Pilot) []string {
	var failures []string
	if p.SchemaVersion != PilotSchemaVersion || p.SnapshotAt.IsZero() || p.LockedAt.Before(p.SnapshotAt) {
		failures = append(failures, "pilot identity and lock time are invalid")
	}
	if len(p.Lanes) != len(PilotAuthorities) {
		failures = append(failures, fmt.Sprintf("pilot has %d lanes; want %d", len(p.Lanes), len(PilotAuthorities)))
	}
	seenLanes := map[string]struct{}{}
	seenItems := map[string]struct{}{}
	for _, lane := range p.Lanes {
		if !slices.Contains(PilotAuthorities, lane.Authority) {
			failures = append(failures, fmt.Sprintf("unknown pilot authority %q", lane.Authority))
		}
		if _, ok := seenLanes[lane.Authority]; ok {
			failures = append(failures, fmt.Sprintf("duplicate pilot authority %q", lane.Authority))
		}
		seenLanes[lane.Authority] = struct{}{}
		if len(lane.Cases) != 10 {
			failures = append(failures, fmt.Sprintf("lane %q has %d cases; want 10", lane.Authority, len(lane.Cases)))
		}
		if !validCeiling(lane.MaxRequests, lane.RequestsUsed) || !validCeiling64(lane.MaxResponseBytes, lane.ResponseBytes) || !validCeiling64(lane.MaxPredictedMediaBytes, lane.PredictedMediaBytes) || !validCeiling64(lane.MaxWallTimeMS, lane.WallTimeMS) {
			failures = append(failures, fmt.Sprintf("lane %q has missing or exceeded ceilings", lane.Authority))
		}
		var predicted int64
		for _, c := range lane.Cases {
			key := lane.Authority + "/" + c.ItemID
			if _, ok := seenItems[key]; ok {
				failures = append(failures, fmt.Sprintf("duplicate pilot item %q", key))
			}
			seenItems[key] = struct{}{}
			if strings.TrimSpace(c.ItemID) == "" || strings.TrimSpace(c.Title) == "" || len(c.RoleHints) == 0 || c.MetadataRetrievedAt.IsZero() || !digest(c.MetadataSHA256, 64) || len(c.RightsAssertions) == 0 || c.Representation.Bytes <= 0 || strings.TrimSpace(c.Representation.Name) == "" || strings.TrimSpace(c.Representation.MIMEType) == "" || !httpsURL(c.ItemURL) || !httpsURL(c.MetadataURL) || !httpsURL(c.Representation.URL) || (c.LicenseURL != "" && !httpsURL(c.LicenseURL)) {
				failures = append(failures, fmt.Sprintf("pilot item %q has incomplete frozen metadata", key))
			}
			if c.MetadataRetrievedAt.After(p.SnapshotAt) {
				failures = append(failures, fmt.Sprintf("pilot item %q was retrieved after the snapshot", key))
			}
			if predicted > lane.MaxPredictedMediaBytes-c.Representation.Bytes {
				predicted = lane.MaxPredictedMediaBytes + 1
			} else {
				predicted += c.Representation.Bytes
			}
		}
		if predicted != lane.PredictedMediaBytes {
			failures = append(failures, fmt.Sprintf("lane %q predicted media bytes do not match its cases", lane.Authority))
		}
	}
	for _, authority := range PilotAuthorities {
		if _, ok := seenLanes[authority]; !ok {
			failures = append(failures, fmt.Sprintf("pilot is missing authority %q", authority))
		}
	}
	slices.Sort(failures)
	return failures
}

func validCeiling(maximum, used int) bool     { return maximum > 0 && used >= 0 && used <= maximum }
func validCeiling64(maximum, used int64) bool { return maximum > 0 && used >= 0 && used <= maximum }

func digest(value string, size int) bool {
	if len(value) != size {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func httpsURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

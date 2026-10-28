package broadcaststation

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"sync"
)

//go:embed stations.tsv.gz
var seedData []byte

// SeedRevision is one shipped station directory. Unlike the taxonomy, the directory is not operator
// data: applying a revision replaces every stored station, so a corrected station ships as a new
// revision rather than an edit. Load is deferred so an install that already holds the revision never
// decompresses the file.
type SeedRevision struct {
	Version int
	Load    func() ([]Station, error)
}

// SeedRevisions returns every shipped revision, oldest first. Only the newest unapplied one matters.
func SeedRevisions() []SeedRevision {
	return []SeedRevision{{Version: 1, Load: shippedSeed}}
}

// Parsed once per process: every store open (each test store included) offers the same revision.
var shippedSeed = sync.OnceValues(func() ([]Station, error) { return DecodeSeed(bytes.NewReader(seedData)) })

// One row per call-sign span, grouped by station; the station columns repeat on every row.
var seedColumns = []string{"station_id", "country", "city", "region", "latitude", "longitude", "call_sign", "from", "to"}

// EncodeSeed writes stations in the seed format. cmd/station-data is its only writer.
func EncodeSeed(w io.Writer, stations []Station) error {
	gz := gzip.NewWriter(w)
	tsv := csv.NewWriter(gz)
	tsv.Comma = '\t'
	if err := tsv.Write(seedColumns); err != nil {
		return err
	}
	for _, s := range stations {
		if err := s.Validate(); err != nil {
			return err
		}
		lat, lon := strconv.FormatFloat(s.Latitude, 'f', 5, 64), strconv.FormatFloat(s.Longitude, 'f', 5, 64)
		for _, c := range s.CallSigns {
			if err := tsv.Write([]string{s.ID, s.Country, s.City, s.Region, lat, lon, c.Sign, c.From, c.To}); err != nil {
				return err
			}
		}
	}
	tsv.Flush()
	if err := tsv.Error(); err != nil {
		return err
	}
	return gz.Close()
}

// DecodeSeed reads the seed format and validates every station.
func DecodeSeed(r io.Reader) ([]Station, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("broadcaststation: open seed: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tsv := csv.NewReader(gz)
	tsv.Comma = '\t'
	tsv.FieldsPerRecord = len(seedColumns)
	tsv.ReuseRecord = true
	if _, err := tsv.Read(); err != nil {
		return nil, fmt.Errorf("broadcaststation: read seed header: %w", err)
	}
	var stations []Station
	for {
		record, err := tsv.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("broadcaststation: read seed: %w", err)
		}
		lat, latErr := strconv.ParseFloat(record[4], 64)
		lon, lonErr := strconv.ParseFloat(record[5], 64)
		if latErr != nil || lonErr != nil {
			return nil, fmt.Errorf("broadcaststation: station %s: malformed coordinates", record[0])
		}
		row := Station{ID: record[0], Country: record[1], City: record[2], Region: record[3], Latitude: lat, Longitude: lon}
		if n := len(stations); n == 0 || stations[n-1].ID != row.ID {
			stations = append(stations, row)
		} else if prev := stations[n-1]; prev.Country != row.Country || prev.City != row.City || prev.Region != row.Region ||
			prev.Latitude != row.Latitude || prev.Longitude != row.Longitude {
			return nil, fmt.Errorf("broadcaststation: station %s: rows disagree about where it is", row.ID)
		}
		last := &stations[len(stations)-1]
		last.CallSigns = append(last.CallSigns, CallSign{Sign: record[6], From: record[7], To: record[8]})
	}
	seen := make(map[string]bool, len(stations))
	for _, s := range stations {
		if seen[s.ID] {
			return nil, fmt.Errorf("broadcaststation: station %s is not contiguous in the seed", s.ID)
		}
		seen[s.ID] = true
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("broadcaststation: %w", err)
		}
	}
	return stations, nil
}

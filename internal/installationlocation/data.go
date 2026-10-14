package installationlocation

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

//go:embed places.tsv.gz
var placesData []byte

//go:embed ip-country.csv.gz
var ipCountryData []byte

var (
	defaultOnce  sync.Once
	defaultIndex *Index
	defaultErr   error
)

// Default returns the immutable generated location index. Parsing is deferred until a location
// surface is used so installs that already have geography do not pay its memory cost at startup.
func Default() (*Index, error) {
	defaultOnce.Do(func() {
		places, err := readPlaces()
		if err != nil {
			defaultErr = err
			return
		}
		defaultIndex, err = NewIndex(places, nil)
		if err != nil {
			defaultErr = err
			return
		}
		defaultErr = readIPRanges(defaultIndex)
		if defaultErr == nil {
			defaultIndex.sortRanges()
		}
	})
	return defaultIndex, defaultErr
}

func readPlaces() ([]Place, error) {
	reader, err := gzip.NewReader(bytes.NewReader(placesData))
	if err != nil {
		return nil, fmt.Errorf("installationlocation: open place index: %w", err)
	}
	defer func() { _ = reader.Close() }()
	tsv := csv.NewReader(reader)
	tsv.Comma = '\t'
	var places []Place
	for {
		record, readErr := tsv.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("installationlocation: read place index: %w", readErr)
		}
		if len(record) != 10 {
			return nil, fmt.Errorf("installationlocation: place row has %d fields", len(record))
		}
		place := Place{ID: record[0], Name: record[1], SearchText: record[2], Country: record[3], CountryName: record[4], Region: record[5]}
		if record[9] != "country" {
			place.Market = record[1]
			place.Latitude, err = strconv.ParseFloat(record[6], 64)
			if err != nil {
				return nil, fmt.Errorf("installationlocation: parse latitude: %w", err)
			}
			place.Longitude, err = strconv.ParseFloat(record[7], 64)
			if err != nil {
				return nil, fmt.Errorf("installationlocation: parse longitude: %w", err)
			}
		}
		place.Population, err = strconv.ParseInt(record[8], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("installationlocation: parse population: %w", err)
		}
		places = append(places, place)
	}
	return places, nil
}

func readIPRanges(index *Index) error {
	reader, err := gzip.NewReader(bytes.NewReader(ipCountryData))
	if err != nil {
		return fmt.Errorf("installationlocation: open IP index: %w", err)
	}
	defer func() { _ = reader.Close() }()
	rows := csv.NewReader(reader)
	for {
		record, readErr := rows.Read()
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("installationlocation: read IP index: %w", readErr)
		}
		if len(record) != 3 {
			return fmt.Errorf("installationlocation: IP row has %d fields", len(record))
		}
		if err := index.addRange(IPRange{From: record[0], To: record[1], Country: record[2]}); err != nil {
			return err
		}
	}
}

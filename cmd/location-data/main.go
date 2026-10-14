// Command location-data builds the deterministic offline Installation-location indexes.
package main

import (
	"archive/zip"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	citiesURL      = "https://download.geonames.org/export/dump/cities15000.zip"
	citiesSHA      = "15b9401f1e3216219bc58474a1d150c1c9e81dfbfb58e6188c96261f94a393db"
	municipalURL   = "https://download.geonames.org/export/dump/allCountries.zip"
	municipalSHA   = "0a90c49c9b4f0c76965469df22d507eb82b80da8e1cef8ec058be2bbdab2c287"
	regionsURL     = "https://download.geonames.org/export/dump/admin1CodesASCII.txt"
	regionsSHA     = "590651498043f674accda2b7f46d21286cda0e290b02f8561c5005eee9a5448c"
	countriesURL   = "https://download.geonames.org/export/dump/countryInfo.txt"
	countriesSHA   = "93bafc525813f22e4711ff9ed6d626343094ce48c26388dc7c49189b3d7d5512"
	dbipURL        = "https://download.db-ip.com/free/dbip-country-lite-2026-09.csv.gz"
	dbipSHA        = "a32bb3c384bd3de60ad9024596aa5b395a6dd5beaa27a7223407cc2edc681d0b"
	geoNamesCredit = "GeoNames data is licensed under CC BY 4.0: https://www.geonames.org/"
	dbipCredit     = "IP Geolocation by DB-IP (September 2026 Lite), licensed under CC BY 4.0: https://db-ip.com"
)

type source struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Credit string `json:"credit"`
}

type output struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Rows   int    `json:"rows"`
}

type metadata struct {
	Version string   `json:"version"`
	Sources []source `json:"sources"`
	Outputs []output `json:"outputs"`
}

func main() {
	outputDir := flag.String("output", ".", "output directory")
	flag.Parse()

	tempDir, err := os.MkdirTemp("", "loomarr-location-data-")
	must(err)
	defer func() { must(os.RemoveAll(tempDir)) }()

	cities := download(tempDir, citiesURL, citiesSHA)
	municipalities := download(tempDir, municipalURL, municipalSHA)
	regions := download(tempDir, regionsURL, regionsSHA)
	countries := download(tempDir, countriesURL, countriesSHA)
	dbip := download(tempDir, dbipURL, dbipSHA)

	countryNames := readCountries(countries)
	regionNames := readRegions(regions)
	placesPath := filepath.Join(*outputDir, "places.tsv.gz")
	placeRows := writePlaces(placesPath, cities, municipalities, countryNames, regionNames)
	ipPath := filepath.Join(*outputDir, "ip-country.csv.gz")
	ipRows := writeIPRanges(ipPath, dbip)

	meta := metadata{
		Version: "2026-09",
		Sources: []source{
			{URL: citiesURL, SHA256: citiesSHA, Credit: geoNamesCredit},
			{URL: municipalURL, SHA256: municipalSHA, Credit: geoNamesCredit},
			{URL: regionsURL, SHA256: regionsSHA, Credit: geoNamesCredit},
			{URL: countriesURL, SHA256: countriesSHA, Credit: geoNamesCredit},
			{URL: dbipURL, SHA256: dbipSHA, Credit: dbipCredit},
		},
		Outputs: []output{
			{File: filepath.Base(placesPath), SHA256: fileSHA(placesPath), Rows: placeRows},
			{File: filepath.Base(ipPath), SHA256: fileSHA(ipPath), Rows: ipRows},
		},
	}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	must(err)
	encoded = append(encoded, '\n')
	must(os.WriteFile(filepath.Join(*outputDir, "location-data.generated.json"), encoded, 0o644))
}

func readRegions(path string) map[string]string {
	file, err := os.Open(path)
	must(err)
	defer func() { must(file.Close()) }()
	out := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) >= 2 {
			out[fields[0]] = fields[1]
		}
	}
	must(scanner.Err())
	return out
}

func download(dir, rawURL, wantSHA string) string {
	client := &http.Client{Timeout: 45 * time.Second}
	response, err := client.Get(rawURL)
	must(err)
	defer func() { must(response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("download %s: %s", rawURL, response.Status))
	}
	path := filepath.Join(dir, filepath.Base(rawURL))
	file, err := os.Create(path)
	must(err)
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(file, hash), response.Body)
	must(err)
	must(file.Close())
	got := hex.EncodeToString(hash.Sum(nil))
	if got != wantSHA {
		panic(fmt.Sprintf("download %s: sha256 %s, want %s", rawURL, got, wantSHA))
	}
	return path
}

func readCountries(path string) map[string]string {
	file, err := os.Open(path)
	must(err)
	defer func() { must(file.Close()) }()
	out := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) > 4 && len(fields[0]) == 2 {
			out[fields[0]] = fields[4]
		}
	}
	must(scanner.Err())
	return out
}

func writePlaces(path, citiesPath, municipalitiesPath string, countries, regions map[string]string) int {
	file, err := os.Create(path)
	must(err)
	defer func() { must(file.Close()) }()
	gz := gzip.NewWriter(file)
	gz.ModTime = time.Unix(0, 0)
	writer := csv.NewWriter(gz)
	writer.Comma = '\t'

	rows := 0
	for code, name := range sortedCountries(countries) {
		must(writer.Write([]string{"country-" + code, name, name, code, name, "", "", "", "0", "country"}))
		rows++
	}

	rows += writeGeoNames(writer, citiesPath, countries, regions, false)
	rows += writeGeoNames(writer, municipalitiesPath, countries, regions, true)
	writer.Flush()
	must(writer.Error())
	must(gz.Close())
	return rows
}

func writeGeoNames(writer *csv.Writer, sourcePath string, countries, regions map[string]string, municipalitiesOnly bool) int {
	archive, err := zip.OpenReader(sourcePath)
	must(err)
	defer func() { must(archive.Close()) }()
	if len(archive.File) != 1 {
		panic(fmt.Sprintf("GeoNames archive contains %d files, want 1", len(archive.File)))
	}
	reader, err := archive.File[0].Open()
	must(err)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	rows := 0
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 15 {
			panic("malformed GeoNames row")
		}
		if municipalitiesOnly {
			population, parseErr := strconv.ParseInt(fields[14], 10, 64)
			if parseErr != nil || fields[6] != "A" || (fields[7] != "ADM3" && fields[7] != "ADM4") || population <= 0 {
				continue
			}
		}
		countryName, ok := countries[fields[8]]
		if !ok {
			continue
		}
		name := fields[1]
		if municipalitiesOnly {
			name = friendlyMunicipalityName(name, fields[8])
		}
		searchText := name
		if !strings.EqualFold(name, fields[1]) {
			searchText += " " + fields[1]
		}
		if fields[2] != "" && !strings.EqualFold(fields[1], fields[2]) {
			searchText += " " + fields[2]
		}
		region := regions[fields[8]+"."+fields[10]]
		if region != "" && !strings.Contains(strings.ToLower(searchText), strings.ToLower(region)) {
			searchText += " " + region
		}
		kind := "city"
		if municipalitiesOnly {
			kind = "municipality"
		}
		must(writer.Write([]string{fields[0], name, searchText, fields[8], countryName, region, fields[4], fields[5], fields[14], kind}))
		rows++
	}
	must(scanner.Err())
	must(reader.Close())
	return rows
}

func friendlyMunicipalityName(name, country string) string {
	if country != "US" {
		return name
	}
	for _, prefix := range []string{"Town of ", "Township of ", "City of ", "Village of ", "Borough of "} {
		if trimmed, ok := strings.CutPrefix(name, prefix); ok && trimmed != "" {
			return trimmed
		}
	}
	return name
}

func sortedCountries(countries map[string]string) func(func(string, string) bool) {
	return func(yield func(string, string) bool) {
		codes := make([]string, 0, len(countries))
		for code := range countries {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			if !yield(code, countries[code]) {
				return
			}
		}
	}
}

func writeIPRanges(path, sourcePath string) int {
	in, err := os.Open(sourcePath)
	must(err)
	defer func() { must(in.Close()) }()
	reader, err := gzip.NewReader(in)
	must(err)
	defer func() { must(reader.Close()) }()
	csvReader := csv.NewReader(reader)

	out, err := os.Create(path)
	must(err)
	defer func() { must(out.Close()) }()
	gz := gzip.NewWriter(out)
	gz.ModTime = time.Unix(0, 0)
	csvWriter := csv.NewWriter(gz)
	rows := 0
	for {
		record, readErr := csvReader.Read()
		if readErr == io.EOF {
			break
		}
		must(readErr)
		if len(record) != 3 || len(record[2]) != 2 {
			panic("malformed DB-IP country row")
		}
		must(csvWriter.Write(record))
		rows++
	}
	csvWriter.Flush()
	must(csvWriter.Error())
	must(gz.Close())
	return rows
}

func fileSHA(path string) string {
	file, err := os.Open(path)
	must(err)
	defer func() { must(file.Close()) }()
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	must(err)
	return hex.EncodeToString(hash.Sum(nil))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

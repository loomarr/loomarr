// Command filler-household-cohort binds the frozen 50-case reference seed to
// the exact outputs produced by a running beta.6 pipeline and selects the first
// 32 Ready, range-playable, duplicate-safe clips in seed order.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerrelease"
	_ "modernc.org/sqlite"
)

type seedArtifact struct {
	SelectedCaseIDs []string `json:"selectedCaseIds"`
}

type familyArtifact struct {
	Families []struct {
		FamilyID string   `json:"familyId"`
		Members  []string `json:"members"`
	} `json:"families"`
}

type mediaAsset struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type clipSidecar struct {
	Loomarr struct {
		OriginalName string `json:"originalName"`
		MediaAssets  struct {
			SourceMaster mediaAsset `json:"sourceMaster"`
			Playback     struct {
				Asset mediaAsset `json:"asset"`
				QC    struct {
					CompleteDecode bool `json:"completeDecode"`
					Seekable       bool `json:"seekable"`
					FastStart      bool `json:"fastStart"`
				} `json:"qc"`
			} `json:"playback"`
		} `json:"mediaAssets"`
	} `json:"loomarr"`
}

type catalogRow struct {
	hash, path, disposition string
}

type outputArtifact struct {
	SchemaVersion  int                           `json:"schema_version"`
	GeneratedAt    time.Time                     `json:"generated_at"`
	SeedSHA256     string                        `json:"seed_sha256"`
	FamiliesSHA256 string                        `json:"families_sha256"`
	Selection      fillerrelease.CohortSelection `json:"selection"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now, http.DefaultClient)) }

func run(args []string, stdout, stderr io.Writer, now func() time.Time, client *http.Client) int {
	flags := flag.NewFlagSet("filler-household-cohort", flag.ContinueOnError)
	flags.SetOutput(stderr)
	databasePath := flags.String("db", "", "running Loomarr SQLite database")
	fillerRoot := flags.String("filler-root", "", "running Loomarr filler directory")
	seedPath := flags.String("seed", "", "frozen 50-case inspection seed JSON")
	familiesPath := flags.String("families", "", "bound duplicate-family JSON")
	baseURL := flags.String("base-url", "", "running Loomarr API base URL")
	cookiePath := flags.String("cookie-file", "", "Netscape cookie file for the authenticated Range probe")
	outPath := flags.String("out", "", "output cohort JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	for name, value := range map[string]string{
		"-base-url": *baseURL, "-cookie-file": *cookiePath, "-db": *databasePath,
		"-families": *familiesPath, "-filler-root": *fillerRoot, "-out": *outPath, "-seed": *seedPath,
	} {
		if strings.TrimSpace(value) == "" {
			_, _ = fmt.Fprintf(stderr, "filler-household-cohort: %s is required\n", name)
			return 2
		}
	}
	artifact, err := assemble(context.Background(), assemblyConfig{
		DatabasePath: *databasePath, FillerRoot: *fillerRoot, SeedPath: *seedPath,
		FamiliesPath: *familiesPath, BaseURL: strings.TrimRight(*baseURL, "/"), CookiePath: *cookiePath,
		GeneratedAt: now().UTC(), Client: client,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-household-cohort:", err)
		return 1
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-household-cohort: encode:", err)
		return 1
	}
	if err := os.WriteFile(*outPath, append(encoded, '\n'), 0o600); err != nil {
		_, _ = fmt.Fprintln(stderr, "filler-household-cohort: write:", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "filler-household-cohort: selected %d exact clips with %d reserves; artifact: %s\n",
		len(artifact.Selection.Selected), len(artifact.Selection.Reserves), *outPath)
	return 0
}

type assemblyConfig struct {
	DatabasePath, FillerRoot, SeedPath, FamiliesPath, BaseURL, CookiePath string
	GeneratedAt                                                           time.Time
	Client                                                                *http.Client
}

func assemble(ctx context.Context, config assemblyConfig) (outputArtifact, error) {
	seedBytes, err := os.ReadFile(config.SeedPath)
	if err != nil {
		return outputArtifact{}, fmt.Errorf("read seed: %w", err)
	}
	var seed seedArtifact
	if err := json.Unmarshal(seedBytes, &seed); err != nil {
		return outputArtifact{}, fmt.Errorf("decode seed: %w", err)
	}
	familyBytes, err := os.ReadFile(config.FamiliesPath)
	if err != nil {
		return outputArtifact{}, fmt.Errorf("read families: %w", err)
	}
	var families familyArtifact
	if err := json.Unmarshal(familyBytes, &families); err != nil {
		return outputArtifact{}, fmt.Errorf("decode families: %w", err)
	}
	familyByCase := map[string]string{}
	for _, family := range families.Families {
		if strings.TrimSpace(family.FamilyID) == "" {
			return outputArtifact{}, fmt.Errorf("duplicate family identity is empty")
		}
		for _, caseID := range family.Members {
			if prior := familyByCase[caseID]; prior != "" {
				return outputArtifact{}, fmt.Errorf("case %q belongs to duplicate families %q and %q", caseID, prior, family.FamilyID)
			}
			familyByCase[caseID] = family.FamilyID
		}
	}
	cookies, err := readCookies(config.CookiePath)
	if err != nil {
		return outputArtifact{}, err
	}
	db, err := sql.Open("sqlite", "file:"+config.DatabasePath+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return outputArtifact{}, fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx, `SELECT c.hash, c.path, p.disposition
FROM clips c JOIN filler_clip_pipeline p ON p.clip_hash = c.hash
WHERE c.source = 'filler-dir' AND c.parent_hash = ''`)
	if err != nil {
		return outputArtifact{}, fmt.Errorf("query pipeline inventory: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byName := map[string]catalogRow{}
	for rows.Next() {
		var row catalogRow
		if err := rows.Scan(&row.hash, &row.path, &row.disposition); err != nil {
			return outputArtifact{}, fmt.Errorf("scan pipeline inventory: %w", err)
		}
		sidecarPath := filepath.Join(config.FillerRoot, strings.TrimSuffix(row.path, filepath.Ext(row.path))+".info.json")
		sidecarBytes, err := os.ReadFile(sidecarPath)
		if err != nil {
			return outputArtifact{}, fmt.Errorf("read sidecar for %s: %w", row.hash, err)
		}
		var sidecar clipSidecar
		if err := json.Unmarshal(sidecarBytes, &sidecar); err != nil {
			return outputArtifact{}, fmt.Errorf("decode sidecar for %s: %w", row.hash, err)
		}
		name := strings.TrimSuffix(sidecar.Loomarr.OriginalName, filepath.Ext(sidecar.Loomarr.OriginalName))
		if name == "" {
			continue
		}
		if _, exists := byName[name]; exists {
			return outputArtifact{}, fmt.Errorf("pipeline inventory repeats original name %q", name)
		}
		byName[name] = row
	}
	if err := rows.Err(); err != nil {
		return outputArtifact{}, fmt.Errorf("read pipeline inventory: %w", err)
	}
	candidates := make([]fillerrelease.CohortCandidate, 0, len(seed.SelectedCaseIDs))
	for _, caseID := range seed.SelectedCaseIDs {
		name := caseID[strings.LastIndex(caseID, "/")+1:]
		row, exists := byName[name]
		if !exists {
			return outputArtifact{}, fmt.Errorf("seed case %q is absent from the pipeline inventory", caseID)
		}
		candidate := fillerrelease.CohortCandidate{
			CaseID: caseID, ClipHash: row.hash, SourceIdentity: sourceIdentity(caseID),
			DuplicateFamilyIdentity: familyByCase[caseID], Disposition: row.disposition,
		}
		if row.disposition == "ready" {
			candidate, err = bindReadyCandidate(ctx, config, cookies, row, candidate)
			if err != nil {
				return outputArtifact{}, fmt.Errorf("bind %s: %w", caseID, err)
			}
		}
		candidates = append(candidates, candidate)
	}
	selection, err := fillerrelease.SelectHouseholdCohort(seed.SelectedCaseIDs, candidates)
	if err != nil {
		return outputArtifact{}, err
	}
	return outputArtifact{
		SchemaVersion: 1, GeneratedAt: config.GeneratedAt, SeedSHA256: hashBytes(seedBytes),
		FamiliesSHA256: hashBytes(familyBytes), Selection: selection,
	}, nil
}

func bindReadyCandidate(ctx context.Context, config assemblyConfig, cookies []*http.Cookie, row catalogRow, candidate fillerrelease.CohortCandidate) (fillerrelease.CohortCandidate, error) {
	sidecarRel := strings.TrimSuffix(row.path, filepath.Ext(row.path)) + ".info.json"
	sidecarPath, err := confinedPath(config.FillerRoot, sidecarRel)
	if err != nil {
		return candidate, err
	}
	sidecarBytes, err := os.ReadFile(sidecarPath)
	if err != nil {
		return candidate, err
	}
	var sidecar clipSidecar
	if err := json.Unmarshal(sidecarBytes, &sidecar); err != nil {
		return candidate, err
	}
	playback := sidecar.Loomarr.MediaAssets.Playback
	if !playback.QC.CompleteDecode || !playback.QC.Seekable || !playback.QC.FastStart {
		return candidate, fmt.Errorf("playback QC is incomplete")
	}
	if playback.Asset.Path != row.path {
		return candidate, fmt.Errorf("playback path %q does not match catalog path %q", playback.Asset.Path, row.path)
	}
	for _, asset := range []mediaAsset{sidecar.Loomarr.MediaAssets.SourceMaster, playback.Asset} {
		assetPath, pathErr := confinedPath(config.FillerRoot, asset.Path)
		if pathErr != nil {
			return candidate, pathErr
		}
		observed, hashErr := hashFile(assetPath)
		if hashErr != nil {
			return candidate, hashErr
		}
		if observed != asset.SHA256 {
			return candidate, fmt.Errorf("asset %q digest is %s, want %s", asset.Path, observed, asset.SHA256)
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, config.BaseURL+"/v1/filler/media/"+row.hash, nil)
	if err != nil {
		return candidate, err
	}
	request.Header.Set("Range", "bytes=0-1023")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err := config.Client.Do(request)
	if err != nil {
		return candidate, fmt.Errorf("range playback: %w", err)
	}
	bytesRead, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 1025))
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusPartialContent {
		return candidate, fmt.Errorf("range playback returned HTTP %d", response.StatusCode)
	}
	if readErr != nil {
		return candidate, fmt.Errorf("read range playback: %w", readErr)
	}
	if closeErr != nil {
		return candidate, fmt.Errorf("close range playback: %w", closeErr)
	}
	if bytesRead == 0 || !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes 0-") {
		return candidate, fmt.Errorf("range playback response is incomplete")
	}
	candidate.SourceMasterSHA256 = sidecar.Loomarr.MediaAssets.SourceMaster.SHA256
	candidate.PlaybackDerivativeSHA256 = playback.Asset.SHA256
	candidate.SidecarSHA256 = hashBytes(sidecarBytes)
	candidate.LineageSHA256 = hashBytes([]byte(strings.Join([]string{
		candidate.CaseID, candidate.ClipHash, candidate.SourceIdentity, candidate.SourceMasterSHA256,
		candidate.PlaybackDerivativeSHA256, candidate.SidecarSHA256,
	}, "\x00")))
	candidate.RangePlayback = true
	return candidate, nil
}

func sourceIdentity(caseID string) string {
	parts := strings.Split(caseID, "/")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[:2], "/")
}

func confinedPath(root, relative string) (string, error) {
	if !filepath.IsLocal(relative) {
		return "", fmt.Errorf("path %q is outside the filler root", relative)
	}
	return filepath.Join(root, relative), nil
}

func readCookies(path string) ([]*http.Cookie, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open cookie file: %w", err)
	}
	defer func() { _ = file.Close() }()
	var cookies []*http.Cookie
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 7 || fields[5] == "" {
			return nil, fmt.Errorf("cookie file contains an invalid row")
		}
		cookies = append(cookies, &http.Cookie{Name: fields[5], Value: fields[6]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read cookie file: %w", err)
	}
	if len(cookies) == 0 {
		return nil, fmt.Errorf("cookie file contains no cookies")
	}
	return cookies, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

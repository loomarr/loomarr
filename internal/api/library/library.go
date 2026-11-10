package library

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/library/scan"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Settings → Library (#2151 decisions 6 and 29): the folders Loomarr reads media from, the path
// map that names them for each authority, and the status strip. Admin-only: folder paths describe
// the household's storage, which members never need.

// LibraryRootService discovers, checks and records scan roots, and runs the scanner over them
// (scan.Library).
type LibraryRootService interface {
	List(ctx context.Context) (librarystore.LibraryRoots, error)
	Discover(ctx context.Context) (librarystore.LibraryRoots, []scan.SourceResult, error)
	Add(ctx context.Context, path string) error
	SetInUse(ctx context.Context, path string, inUse bool) error
	SetMapping(ctx context.Context, authority, authorityPath, local string) error

	Enabled() bool
	Progress() scan.Progress
	Importing() scan.ProgressSnapshot
	Counts(ctx context.Context) (librarystore.LibraryFileCounts, error)
	Rescan() error
	RefreshEverything(ctx context.Context) error
	ConfirmRemovals(ctx context.Context, root string) error

	Items(ctx context.Context, q librarystore.LibraryItemQuery) (librarystore.LibraryItemPage, error)
	Item(ctx context.Context, id string) (scan.ItemDetail, error)
	UpdateDetails(ctx context.Context, id string) error
	CheckFiles(ctx context.Context, id string) error
	RemoveExtraCopy(ctx context.Context, id, path string) error

	Needs(ctx context.Context) ([]scan.Need, error)
	Problems(ctx context.Context) ([]librarystore.LibraryItemSummary, error)
	NeedCount(ctx context.Context) (int, error)
	Link(ctx context.Context, itemID string, id librarystore.LibraryID) error
	Ignore(ctx context.Context, itemID string) error
	Unanswer(ctx context.Context, itemID string) error
	SearchTitles(ctx context.Context, term, kind string) ([]scan.SearchHit, error)
	// Episode orders (#2151 decision 53).
	OrderNeeds(ctx context.Context) ([]scan.OrderNeed, error)
	ChooseOrder(ctx context.Context, seriesID string, choice librarystore.LibraryOrderChoice, orderID string) error
	// Collections (#2151 step 12, decision 28).
	Collections(ctx context.Context) ([]scan.Collection, error)
	Collection(ctx context.Context, id string) (scan.Collection, error)
	CreateCollection(ctx context.Context, name, description string, items []string) (string, error)
	UpdateCollection(ctx context.Context, id, name, description string, items []string) error
	DeleteCollection(ctx context.Context, id string) error
	Sets(ctx context.Context) ([]scan.Set, error)
	CopySet(ctx context.Context, id string) (string, error)
}

func (s *Handlers) registerLibrary(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "listLibraryRoots", Method: http.MethodGet, Path: "/v1/library/roots",
		Summary:     "List media folders",
		Description: "The folders Loomarr reads media from, where each was found, its last readability check, and the path map.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.listLibraryRoots)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "discoverLibraryRoots", Method: http.MethodPost, Path: "/v1/library/roots/discover",
		Summary: "Look for media folders again",
		Description: "Asks the media server, Sonarr and Radarr for their folders, finds each one locally, and checks every folder " +
			"can be opened. A source that fails keeps the folders it reported last time.",
		Tags: []string{"library"},
	}, httpcore.RoleAdmin), s.discoverLibraryRoots)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "addLibraryRoot", Method: http.MethodPost, Path: "/v1/library/roots",
		Summary:     "Add a media folder by hand",
		Description: "Adds a folder by its path on this machine and starts using it. 422 when the path is not absolute or Loomarr cannot open it.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.addLibraryRoot)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "setLibraryRootInUse", Method: http.MethodPut, Path: "/v1/library/roots/in-use",
		Summary:     "Use or stop using a media folder",
		Description: "Confirms a folder for scanning, or releases it. Releasing a folder added by hand removes it.",
		Tags:        []string{"library"},
	}, httpcore.RoleAdmin), s.setLibraryRootInUse)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "setLibraryPathMapping", Method: http.MethodPut, Path: "/v1/library/path-map",
		Summary: "Map another app's folder to a folder here",
		Description: "Points one folder, as the media server, Sonarr or Radarr names it, at a folder on this machine. " +
			"An empty localPath hands the row back to discovery.",
		Tags: []string{"library"},
	}, httpcore.RoleAdmin), s.setLibraryPathMapping)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "libraryStatus", Method: http.MethodGet, Path: "/v1/library/status",
		Summary: "Library status",
		Description: "The status strip: whether any folder is in use, whether every folder in use opened at its last check, " +
			"what the scanner is doing, and any removal waiting for the maintainer.",
		Tags: []string{"library"},
	}, httpcore.RoleAdmin), s.libraryStatus)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "rescanLibrary", Method: http.MethodPost, Path: "/v1/library/scan",
		Summary:     "Check the media folders again",
		Description: "Starts a scan of every folder in use now. 409 when the scanner is turned off.",
		Tags:        []string{"library"}, DefaultStatus: http.StatusAccepted,
	}, httpcore.RoleAdmin), s.rescanLibrary)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "refreshLibrary", Method: http.MethodPost, Path: "/v1/library/refresh",
		Summary: "Refresh everything in the library",
		Description: "Writes every file again on a scan started now, and fetches every title's catalog record again " +
			"in the background. Titles keep playing meanwhile. 409 when the scanner is turned off.",
		Tags: []string{"library"}, DefaultStatus: http.StatusAccepted,
	}, httpcore.RoleAdmin), s.refreshLibrary)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "confirmLibraryRemoval", Method: http.MethodPost, Path: "/v1/library/roots/confirm-removal",
		Summary: "Remove the titles a scan no longer found",
		Description: "Applies a held removal: a scan that would remove many titles from one folder waits for this. " +
			"404 when nothing is waiting for that folder; 409 when the scanner is turned off.",
		Tags: []string{"library"}, DefaultStatus: http.StatusAccepted,
	}, httpcore.RoleAdmin), s.confirmLibraryRemoval)
	s.registerLibraryHooks(api)
	s.registerLibraryItems(api)
	s.registerLibraryNeeds(api)
	s.registerLibraryCollections(api)
}

type libraryFolderNameDTO struct {
	Authority string `json:"authority" enum:"emby,jellyfin,sonarr,radarr" doc:"The app that reported the folder"`
	Path      string `json:"path" doc:"The folder as that app names it"`
}

type libraryRootDTO struct {
	Path        string                 `json:"path" doc:"The folder on this machine"`
	InUse       bool                   `json:"inUse" doc:"The maintainer confirmed it for scanning"`
	Readable    bool                   `json:"readable" doc:"Loomarr opened it at the last check"`
	CheckedAt   *time.Time             `json:"checkedAt,omitempty" doc:"The last check; absent when never checked"`
	AddedByHand bool                   `json:"addedByHand"`
	FoundIn     []libraryFolderNameDTO `json:"foundIn" doc:"Every app that names this folder, and what it calls it"`
}

type libraryPathMappingDTO struct {
	Authority     string `json:"authority" enum:"emby,jellyfin,sonarr,radarr"`
	AuthorityPath string `json:"authorityPath" doc:"The folder as the app names it"`
	LocalPath     string `json:"localPath" doc:"The folder on this machine; empty when none was found"`
	ByHand        bool   `json:"byHand" doc:"Set by the maintainer; discovery leaves it alone"`
}

type librarySourceDTO struct {
	Authority string `json:"authority" enum:"emby,jellyfin,sonarr,radarr"`
	State     string `json:"state" enum:"ok,skipped,failed" doc:"skipped: not set up"`
	Error     string `json:"error,omitempty"`
}

type libraryRootsBody struct {
	Roots   []libraryRootDTO        `json:"roots"`
	PathMap []libraryPathMappingDTO `json:"pathMap"`
	// Sources is set only by a discovery pass.
	Sources []librarySourceDTO `json:"sources,omitempty" doc:"How each app answered; only on a discovery pass"`
}

type libraryRootsOutput struct {
	Body libraryRootsBody
}

func libraryRootsView(all librarystore.LibraryRoots, results []scan.SourceResult) *libraryRootsOutput {
	out := &libraryRootsOutput{Body: libraryRootsBody{
		Roots: make([]libraryRootDTO, 0, len(all.Roots)), PathMap: make([]libraryPathMappingDTO, 0, len(all.Paths)),
	}}
	names := map[string][]libraryFolderNameDTO{}
	for _, p := range all.Paths {
		out.Body.PathMap = append(out.Body.PathMap, libraryPathMappingDTO{
			Authority: p.Authority, AuthorityPath: p.AuthorityPath, LocalPath: p.RootPath, ByHand: p.ByHand,
		})
		if p.RootPath != "" {
			names[p.RootPath] = append(names[p.RootPath], libraryFolderNameDTO{Authority: p.Authority, Path: p.AuthorityPath})
		}
	}
	for _, r := range all.Roots {
		dto := libraryRootDTO{Path: r.Path, InUse: r.InUse, Readable: r.Readable, AddedByHand: r.ByHand, FoundIn: names[r.Path]}
		if dto.FoundIn == nil {
			dto.FoundIn = []libraryFolderNameDTO{}
		}
		if !r.CheckedAt.IsZero() {
			checked := r.CheckedAt
			dto.CheckedAt = &checked
		}
		out.Body.Roots = append(out.Body.Roots, dto)
	}
	for _, res := range results {
		out.Body.Sources = append(out.Body.Sources, librarySourceDTO{Authority: res.Authority, State: res.State, Error: res.Error})
	}
	return out
}

func (s *Handlers) libraryRootsService() (LibraryRootService, error) {
	if s.libraryRoots == nil {
		return nil, httpcore.FeatureNotConfigured("Media folders unavailable", "The library scanner is not set up on this server.")
	}
	return s.libraryRoots, nil
}

func (s *Handlers) listLibraryRoots(ctx context.Context, _ *struct{}) (*libraryRootsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	all, err := svc.List(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read media folders", "Try again in a moment.", err)
	}
	return libraryRootsView(all, nil), nil
}

func (s *Handlers) discoverLibraryRoots(ctx context.Context, _ *struct{}) (*libraryRootsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	all, results, err := svc.Discover(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't look for media folders", "Try again in a moment.", err)
	}
	return libraryRootsView(all, results), nil
}

type addLibraryRootInput struct {
	Body struct {
		Path string `json:"path" minLength:"1" maxLength:"4096" doc:"The folder's absolute path on this machine"`
	}
}

func (s *Handlers) addLibraryRoot(ctx context.Context, in *addLibraryRootInput) (*libraryRootsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	switch err := svc.Add(ctx, in.Body.Path); {
	case errors.Is(err, scan.ErrNotAbsolute):
		return nil, httpcore.Unprocessable("That isn't a full path", "Type the folder's full path, starting with /.")
	case errors.Is(err, scan.ErrUnreadable):
		return nil, httpcore.Unprocessable("Loomarr can't open that folder", "Check the folder exists here and Loomarr is allowed to read it.")
	case err != nil:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't add the folder", "Try again in a moment.", err)
	}
	return s.listLibraryRoots(ctx, nil)
}

type setLibraryRootInUseInput struct {
	Body struct {
		Path  string `json:"path" minLength:"1" maxLength:"4096"`
		InUse bool   `json:"inUse"`
	}
}

func (s *Handlers) setLibraryRootInUse(ctx context.Context, in *setLibraryRootInUseInput) (*libraryRootsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	switch err := svc.SetInUse(ctx, in.Body.Path, in.Body.InUse); {
	case errors.Is(err, store.ErrNotFound):
		return nil, httpcore.NotFound("No such folder", "Loomarr doesn't know that folder. Look for folders again.")
	case err != nil:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't change the folder", "Try again in a moment.", err)
	}
	return s.listLibraryRoots(ctx, nil)
}

type setLibraryPathMappingInput struct {
	Body struct {
		Authority     string `json:"authority" enum:"emby,jellyfin,sonarr,radarr"`
		AuthorityPath string `json:"authorityPath" minLength:"1" maxLength:"4096"`
		LocalPath     string `json:"localPath" maxLength:"4096" doc:"Empty hands the row back to discovery"`
	}
}

func (s *Handlers) setLibraryPathMapping(ctx context.Context, in *setLibraryPathMappingInput) (*libraryRootsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	switch err := svc.SetMapping(ctx, in.Body.Authority, in.Body.AuthorityPath, in.Body.LocalPath); {
	case errors.Is(err, scan.ErrNotAbsolute):
		return nil, httpcore.Unprocessable("That isn't a full path", "Type the folder's full path, starting with /.")
	case errors.Is(err, store.ErrNotFound):
		return nil, httpcore.NotFound("No such folder", "That app hasn't reported this folder. Look for folders again.")
	case err != nil:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't change the folder map", "Try again in a moment.", err)
	}
	return s.listLibraryRoots(ctx, nil)
}

// ScanProgress is how far a running import is: the earliest of its three steps still running.
// It is the status endpoint's scan and, whole, the library_scan live frame, so the two never
// differ in shape.
type ScanProgress struct {
	Step       string `json:"step" enum:"idle,walking,reading,lookingUp" doc:"The earliest step running: walking looks through the folders, reading reads each new file, lookingUp looks up each new title's details; idle when none runs"`
	StepNumber int    `json:"stepNumber" minimum:"0" maximum:"3" doc:"The step's number, 1 to 3; 0 when idle"`
	Done       int    `json:"done" doc:"How much of the step is done: top-level folder entries, files or titles"`
	Total      int    `json:"total" doc:"How much the step has to do; 0 while it is still being counted"`
	Current    string `json:"current,omitempty" doc:"What the step is on now: a folder, a file or a title; absent when unknown or paused"`
	// EtaSeconds is absent until the step has run 30 s and done 2% of its total, and while paused.
	EtaSeconds *int `json:"etaSeconds,omitempty" minimum:"0" doc:"Seconds the step has left; absent until it can be told, and while paused"`
	Paused     bool `json:"paused" doc:"The step waits while someone is watching"`
	Quiet      bool `json:"quiet" doc:"A pass nobody asked for, with nothing to read or look up: shown as one small line"`
}

// ScanProgressOf is the wire shape of p.
func ScanProgressOf(p scan.ProgressSnapshot) ScanProgress {
	out := ScanProgress{
		Step: "idle", StepNumber: p.Number, Done: p.Done, Total: p.Total, Current: p.Current,
		Paused: p.Paused, Quiet: p.Quiet,
	}
	if p.Step != "" {
		out.Step = string(p.Step)
	}
	if p.HasETA {
		eta := int(math.Ceil(p.ETA.Seconds()))
		out.EtaSeconds = &eta
	}
	return out
}

type libraryScanDTO struct {
	Enabled bool `json:"enabled" doc:"library.scanner.enabled: false stops every scan"`
	ScanProgress
	// ArrIncomplete is true when Sonarr or Radarr did not answer the last scan.
	ArrIncomplete bool       `json:"arrIncomplete" doc:"Sonarr or Radarr didn't answer the last scan"`
	LastScanAt    *time.Time `json:"lastScanAt,omitempty" doc:"When the last scan finished; absent before the first"`
}

type libraryHeldDTO struct {
	Path     string `json:"path" doc:"The folder whose scan is waiting"`
	Movies   int    `json:"movies" doc:"Movies the scan no longer found"`
	Episodes int    `json:"episodes" doc:"Episodes the scan no longer found"`
	Other    int    `json:"other" doc:"Extras the scan no longer found"`
}

type libraryStatusOutput struct {
	Body struct {
		Health    string `json:"health" enum:"needs_folders,folder_offline,ok" doc:"needs_folders: none in use; folder_offline: one in use didn't open"`
		InUse     int    `json:"inUse" doc:"Folders in use"`
		Offline   int    `json:"offline" doc:"Folders in use that didn't open at their last check"`
		ToConfirm int    `json:"toConfirm" doc:"Readable folders found but not yet in use"`

		Scan           libraryScanDTO   `json:"scan"`
		Files          int              `json:"files" doc:"Media files found in folders in use"`
		HeadersPending int              `json:"headersPending" doc:"Files not read yet"`
		Problems       int              `json:"problems" doc:"Files Loomarr skips, such as disc images"`
		NeedsYou       int              `json:"needsYou" doc:"Everything Needs you holds: titles Loomarr can't tell what they are, and titles that need a look"`
		Held           []libraryHeldDTO `json:"held" doc:"Removals waiting for the maintainer, one per folder"`
		Hooks          []libraryHookDTO `json:"hooks" doc:"Sonarr's and Radarr's webhooks to Loomarr"`
	}
}

func (s *Handlers) libraryStatus(ctx context.Context, _ *struct{}) (*libraryStatusOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	all, err := svc.List(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the library status", "Try again in a moment.", err)
	}
	counts, err := svc.Counts(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the library status", "Try again in a moment.", err)
	}
	needsYou, err := svc.NeedCount(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the library status", "Try again in a moment.", err)
	}
	st := scan.StatusOf(all)
	out := &libraryStatusOutput{}
	out.Body.NeedsYou = needsYou
	b := &out.Body
	b.Health, b.InUse, b.Offline, b.ToConfirm = string(st.Health), st.InUse, st.Offline, st.ToConfirm
	b.Files, b.HeadersPending, b.Problems = counts.Files, counts.HeadersPending, counts.Problems
	b.Held = heldView(counts.Held)
	b.Hooks = s.libraryHooksView()

	p := svc.Progress()
	b.Scan = libraryScanDTO{
		Enabled: svc.Enabled(), ScanProgress: ScanProgressOf(svc.Importing()), ArrIncomplete: p.ArrIncomplete,
	}
	if !p.CompletedAt.IsZero() {
		at := p.CompletedAt
		b.Scan.LastScanAt = &at
	}
	return out, nil
}

// heldView groups held counts by folder, in folder order.
func heldView(counts []librarystore.LibraryHeldCount) []libraryHeldDTO {
	out := []libraryHeldDTO{}
	for _, c := range counts {
		if len(out) == 0 || out[len(out)-1].Path != c.Root {
			out = append(out, libraryHeldDTO{Path: c.Root})
		}
		h := &out[len(out)-1]
		switch c.Kind {
		case "movie":
			h.Movies += c.Count
		case "episode":
			h.Episodes += c.Count
		default:
			h.Other += c.Count
		}
	}
	return out
}

func (s *Handlers) rescanLibrary(ctx context.Context, _ *struct{}) (*libraryStatusOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	if err := svc.Rescan(); errors.Is(err, scan.ErrScannerOff) {
		return nil, httpcore.Conflict("The scanner is turned off", "Turn library.scanner.enabled back on to scan your folders.")
	} else if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't start a scan", "Try again in a moment.", err)
	}
	return s.libraryStatus(ctx, nil)
}

func (s *Handlers) refreshLibrary(ctx context.Context, _ *struct{}) (*libraryStatusOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	if err := svc.RefreshEverything(ctx); errors.Is(err, scan.ErrScannerOff) {
		return nil, httpcore.Conflict("The scanner is turned off", "Turn library.scanner.enabled back on to scan your folders.")
	} else if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't start the refresh", "Try again in a moment.", err)
	}
	return s.libraryStatus(ctx, nil)
}

type confirmLibraryRemovalInput struct {
	Body struct {
		Path string `json:"path" minLength:"1" maxLength:"4096" doc:"The folder whose removal to apply"`
	}
}

func (s *Handlers) confirmLibraryRemoval(ctx context.Context, in *confirmLibraryRemovalInput) (*libraryStatusOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	switch err := svc.ConfirmRemovals(ctx, in.Body.Path); {
	case errors.Is(err, scan.ErrNothingHeld):
		return nil, httpcore.NotFound("Nothing is waiting", "No removal is waiting for that folder.")
	case errors.Is(err, scan.ErrScannerOff):
		return nil, httpcore.Conflict("The scanner is turned off", "Turn library.scanner.enabled back on to scan your folders.")
	case err != nil:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't remove them", "Try again in a moment.", err)
	}
	return s.libraryStatus(ctx, nil)
}

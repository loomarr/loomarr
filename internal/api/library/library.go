package library

import (
	"context"
	"errors"
	"math"
	"net/http"
	"path"
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
	Replace(ctx context.Context, path string) error
	Browse(ctx context.Context, path string) (scan.Browsed, error)
	SetInUse(ctx context.Context, path string, inUse bool) error
	SetMapping(ctx context.Context, authority, authorityPath, local string) error

	Enabled() bool
	Progress() scan.Progress
	Importing() scan.ProgressSnapshot
	Counts(ctx context.Context) (librarystore.LibraryFileCounts, error)
	HeldTitles(ctx context.Context, root string) ([]string, int, error)
	Skipped(ctx context.Context) ([]librarystore.LibrarySkipped, error)
	Details(ctx context.Context) (scan.Details, error)
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
		Summary: "Add a media folder by hand",
		Description: "Adds a folder by its path on this machine and starts using it. 422 when the path is not absolute or Loomarr cannot open it. " +
			"409 when it is inside a folder in use, or holds folders in use and replace is not set; with replace, it is used in their place.",
		Tags: []string{"library"},
	}, httpcore.RoleAdmin), s.addLibraryRoot)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "browseLibraryFolders", Method: http.MethodGet, Path: "/v1/library/browse",
		Summary: "List the folders in a folder, to pick one",
		Description: "With no path, the folders the apps name, then this machine's mounts. With a path under one of them, the folders in it, " +
			"without what the scan never reads. Each folder's summary is counted afterwards and streamed as `library_folder` frames on " +
			"/v1/events; one counted in the last few minutes comes with the list. 403 for a path outside every mount; 422 when it is " +
			"not absolute or can't be opened.",
		Tags: []string{"library"},
	}, httpcore.RoleAdmin), s.browseLibraryFolders)
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
	Empty       bool                   `json:"empty" doc:"Loomarr opened it and found nothing in it at the last check; usually a drive or share that isn't connected"`
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
		dto := libraryRootDTO{Path: r.Path, InUse: r.InUse, Readable: r.Readable, Empty: r.Empty, AddedByHand: r.ByHand, FoundIn: names[r.Path]}
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
		// Replace uses the folder in place of the folders in use inside it.
		Replace bool `json:"replace,omitempty" doc:"Use it in place of the folders in use inside it"`
	}
}

func (s *Handlers) addLibraryRoot(ctx context.Context, in *addLibraryRootInput) (*libraryRootsOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	add := svc.Add
	if in.Body.Replace {
		add = svc.Replace
	}
	var overlap *scan.RootOverlap
	switch err := add(ctx, in.Body.Path); {
	case errors.Is(err, scan.ErrNotAbsolute):
		return nil, httpcore.Unprocessable("That isn't a full path", "Type the folder's full path, starting with /.")
	case errors.Is(err, scan.ErrUnreadable):
		return nil, httpcore.Unprocessable("Loomarr can't open that folder", "Check the folder exists here and Loomarr is allowed to read it.")
	case errors.As(err, &overlap) && overlap.Within != "":
		return nil, httpcore.Conflict("Already part of "+path.Base(overlap.Within),
			"Loomarr already reads this folder as part of "+overlap.Within+".")
	case errors.As(err, &overlap):
		return nil, httpcore.Conflict("This folder holds folders Loomarr already reads",
			"Use it in their place, and Loomarr reads everything in it as one folder.")
	case err != nil:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't add the folder", "Try again in a moment.", err)
	}
	return s.listLibraryRoots(ctx, nil)
}

// FolderSummary is what one look inside a folder found, for the folder browser: the same shape in
// GET /v1/library/browse and in each `library_folder` frame, which carries one whole.
type FolderSummary struct {
	Path    string `json:"path" doc:"The folder on this machine"`
	Folders int    `json:"folders" doc:"The folders in it the scan would walk"`
	Videos  int    `json:"videos" doc:"The videos directly in it"`
	State   string `json:"state" enum:"counted,slow,unreadable" doc:"slow: it didn't answer in time; unreadable: Loomarr couldn't open it"`
}

// FolderSummaryOf is s on the wire.
func FolderSummaryOf(s scan.FolderSummary) FolderSummary {
	return FolderSummary{Path: s.Path, Folders: s.Folders, Videos: s.Videos, State: string(s.State)}
}

type browseFolderDTO struct {
	Path   string   `json:"path" doc:"The folder on this machine"`
	Name   string   `json:"name" doc:"What its row says: its own name, or its whole path at the top"`
	Named  bool     `json:"named" doc:"An app names it"`
	InUse  bool     `json:"inUse" doc:"Loomarr reads it"`
	Within string   `json:"within,omitempty" doc:"The folder in use that holds it"`
	Holds  []string `json:"holds" doc:"The folders in use inside it"`
	// Summary is absent until it is counted; a library_folder frame brings it.
	Summary *FolderSummary `json:"summary,omitempty" doc:"Absent until counted; a library_folder frame brings it"`
}

type browseLibraryFoldersInput struct {
	Path string `query:"path" maxLength:"4096" doc:"The folder to list; empty lists the top"`
}

type browseLibraryFoldersOutput struct {
	Body struct {
		Path    string            `json:"path" doc:"The folder listed; empty for the top"`
		Parent  string            `json:"parent" doc:"The folder above; empty when that is the top"`
		InUse   bool              `json:"inUse" doc:"Loomarr reads the folder listed"`
		Within  string            `json:"within,omitempty" doc:"The folder in use that holds the folder listed"`
		Holds   []string          `json:"holds" doc:"The folders in use inside the folder listed"`
		Folders []browseFolderDTO `json:"folders"`
	}
}

func (s *Handlers) browseLibraryFolders(ctx context.Context, in *browseLibraryFoldersInput) (*browseLibraryFoldersOutput, error) {
	svc, err := s.libraryRootsService()
	if err != nil {
		return nil, err
	}
	b, err := svc.Browse(ctx, in.Path)
	switch {
	case errors.Is(err, scan.ErrNotAbsolute):
		return nil, httpcore.Unprocessable("That isn't a full path", "A folder's full path starts with /.")
	case errors.Is(err, scan.ErrNotBrowsable):
		return nil, huma.Error403Forbidden("That folder is outside this machine's drives and shares")
	case errors.Is(err, scan.ErrUnreadable):
		return nil, httpcore.Unprocessable("Loomarr can't open that folder", "Check the drive or share is connected and Loomarr is allowed to read it.")
	case err != nil:
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't list the folders", "Try again in a moment.", err)
	}
	out := &browseLibraryFoldersOutput{}
	out.Body.Path, out.Body.Parent = b.Path, b.Parent
	out.Body.InUse, out.Body.Within, out.Body.Holds = b.InUse, b.Within, b.Holds
	if out.Body.Holds == nil {
		out.Body.Holds = []string{}
	}
	out.Body.Folders = make([]browseFolderDTO, 0, len(b.Folders))
	for _, f := range b.Folders {
		dto := browseFolderDTO{Path: f.Path, Name: f.Name, Named: f.Named, InUse: f.InUse, Within: f.Within, Holds: f.Holds}
		if dto.Holds == nil {
			dto.Holds = []string{}
		}
		if f.Summary != nil {
			sum := FolderSummaryOf(*f.Summary)
			dto.Summary = &sum
		}
		out.Body.Folders = append(out.Body.Folders, dto)
	}
	return out, nil
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

// libraryDetailsDTO is how the titles' details are getting on: how many wait and when the next try
// is. A key problem is never the household's to fix (a release build ships its own key), so it is
// said only in the server's log, never here.
type libraryDetailsDTO struct {
	Waiting     int        `json:"waiting" doc:"Movies and series whose details haven't been looked up yet"`
	Unreachable bool       `json:"unreachable" doc:"The catalog didn't answer the last lookup"`
	RetryAt     *time.Time `json:"retryAt,omitempty" doc:"When looking up is tried again; absent when nothing is waiting on a retry"`
}

type libraryScanDTO struct {
	Enabled bool `json:"enabled" doc:"library.scanner.enabled: false stops every scan"`
	ScanProgress
	// ArrIncomplete is true when Sonarr or Radarr did not answer the last scan.
	ArrIncomplete bool       `json:"arrIncomplete" doc:"Sonarr or Radarr didn't answer the last scan"`
	LastScanAt    *time.Time `json:"lastScanAt,omitempty" doc:"When the last scan finished; absent before the first"`
}

type libraryHeldDTO struct {
	Path       string   `json:"path" doc:"The folder whose scan is waiting"`
	Movies     int      `json:"movies" doc:"Movies the scan no longer found"`
	Episodes   int      `json:"episodes" doc:"Episodes the scan no longer found"`
	Other      int      `json:"other" doc:"Extras the scan no longer found"`
	Titles     []string `json:"titles" doc:"A few of the titles whose files it no longer found, by name: a show once for all its episodes"`
	TitleCount int      `json:"titleCount" doc:"How many titles those files are in all"`
}

// librarySkippedDTO is one reason the scan skips files, with its files.
type librarySkippedDTO struct {
	Reason string   `json:"reason" enum:"broken_link,link_outside,disc_image,identity_conflict,invalid_facts" doc:"Why: broken_link, a shortcut whose file is gone; link_outside, a shortcut to a file outside the media folders; disc_image, a whole disc's image; identity_conflict, a file that claims another's identity; invalid_facts, a file whose details were refused"`
	Total  int      `json:"total" doc:"How many files are skipped for this reason"`
	Paths  []string `json:"paths" doc:"The first of them, by path"`
}

type libraryStatusOutput struct {
	Body struct {
		Health    string `json:"health" enum:"needs_folders,folder_offline,folder_empty,ok" doc:"needs_folders: none in use; folder_offline: one in use didn't open; folder_empty: they all opened, but one held nothing"`
		InUse     int    `json:"inUse" doc:"Folders in use"`
		Offline   int    `json:"offline" doc:"Folders in use that didn't open at their last check"`
		Empty     int    `json:"empty" doc:"Folders in use that opened with nothing in them at their last check"`
		ToConfirm int    `json:"toConfirm" doc:"Readable folders found but not yet in use"`

		Scan           libraryScanDTO      `json:"scan"`
		Files          int                 `json:"files" doc:"Media files found in folders in use"`
		HeadersPending int                 `json:"headersPending" doc:"Files not read yet"`
		Problems       int                 `json:"problems" doc:"Files Loomarr skips, such as disc images"`
		Skipped        []librarySkippedDTO `json:"skipped" doc:"The files Loomarr skips, by why, each reason with its count and its first few paths"`
		NeedsYou       int                 `json:"needsYou" doc:"Everything Needs you holds: titles Loomarr can't tell what they are, and titles that need a look"`
		Held           []libraryHeldDTO    `json:"held" doc:"Removals waiting for the maintainer, one per folder"`
		Unreadable     []string            `json:"unreadable" doc:"Files the scan can't keep because their names aren't UTF-8, by path, each as near its name as text allows: to rename"`
		Hooks          []libraryHookDTO    `json:"hooks" doc:"Sonarr's and Radarr's webhooks to Loomarr"`
		Details        libraryDetailsDTO   `json:"details" doc:"How the titles' details are getting on"`
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
	details, err := svc.Details(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the library status", "Try again in a moment.", err)
	}
	st := scan.StatusOf(all)
	out := &libraryStatusOutput{}
	out.Body.NeedsYou = needsYou
	b := &out.Body
	b.Health, b.InUse, b.Offline, b.Empty, b.ToConfirm = string(st.Health), st.InUse, st.Offline, st.Empty, st.ToConfirm
	b.Files, b.HeadersPending, b.Problems = counts.Files, counts.HeadersPending, counts.Problems
	skipped, err := svc.Skipped(ctx)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the library status", "Try again in a moment.", err)
	}
	b.Skipped = make([]librarySkippedDTO, 0, len(skipped))
	for _, sk := range skipped {
		b.Skipped = append(b.Skipped, librarySkippedDTO{Reason: sk.Problem, Total: sk.Total, Paths: sk.Paths})
	}
	b.Held = heldView(counts.Held)
	for i := range b.Held {
		h := &b.Held[i]
		if h.Titles, h.TitleCount, err = svc.HeldTitles(ctx, h.Path); err != nil {
			return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read the library status", "Try again in a moment.", err)
		}
	}
	b.Details = libraryDetailsDTO{Waiting: details.Waiting, Unreachable: details.State == scan.LookupsUnreachable}
	if !details.RetryAt.IsZero() {
		at := details.RetryAt
		b.Details.RetryAt = &at
	}
	b.Hooks = s.libraryHooksView()

	p := svc.Progress()
	b.Unreadable = []string{}
	for _, r := range p.Roots {
		for _, rel := range r.Unreadable {
			b.Unreadable = append(b.Unreadable, path.Join(r.Root, rel))
		}
	}
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

import * as searchApi from "@loomarr/api/endpoints/search";
import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { SearchCandidate } from "@loomarr/api/models/searchCandidate";
import { unwrap } from "@loomarr/api/unwrap";
import { provisionKey } from "@loomarr/core/provision";
import { LoaderCircle, Plus, RotateCcw } from "lucide-react";
import { type ReactNode, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { MovieCollectionChoices, movieCollectionKeys } from "@/suggest/movie-collection-choices";
import { friendlyTitleRationale } from "@/suggest/suggestion-language";
import { SearchCommand } from "../../shell";
import { episodeSelectionLabel } from "../episode-selection-label";
import type { ProposalEditProps } from "./proposal-edit.type";

type PickKind = "ready" | "missing";
type Keyed = { item: ProposalItem; key: string; kind: PickKind };

const mediaLabel = (item: ProposalItem) => (item.mediaType === "series" ? "Series" : "Movie");

const itemDetails = (item: ProposalItem) =>
  [mediaLabel(item), item.officialRating].filter(Boolean).join(" · ");

const stateLabel = (kind: PickKind, selected: boolean) => {
  switch (kind) {
    case "ready":
      return { text: "In your library", variant: "lock" as const };
    default:
      return selected
        ? { text: "Will be added", variant: "tune" as const }
        : { text: "Not in your library", variant: "neutral" as const };
  }
};

const seasonLabel = (item: ProposalItem) => {
  const lo = item.seasonMin ?? 0;
  const hi = item.seasonMax ?? 0;
  if (lo <= 0 && hi <= 0) return null;
  if (lo > 0 && hi > 0) return lo === hi ? `Season ${lo}` : `Seasons ${lo}–${hi}`;
  return lo > 0 ? `From season ${lo}` : `Through season ${hi}`;
};

const proposalItemFromCandidate = (candidate: SearchCandidate): ProposalItem => ({
  name: candidate.name,
  mediaType: candidate.mediaType,
  inLibrary: candidate.inLibrary,
  ...(candidate.libraryItemId ? { libraryItemId: candidate.libraryItemId } : {}),
  ...(candidate.year ? { year: candidate.year } : {}),
  ...(candidate.tmdbId ? { tmdbId: candidate.tmdbId } : {}),
  ...(candidate.tvdbId ? { tvdbId: candidate.tvdbId } : {}),
  ...(candidate.genres ? { genres: candidate.genres } : {}),
  ...(candidate.officialRating ? { officialRating: candidate.officialRating } : {}),
  ...(candidate.overview ? { overview: candidate.overview } : {}),
  ...(candidate.runtimeMinutes ? { runtimeMinutes: candidate.runtimeMinutes } : {}),
});

const PickRow = ({
  pick,
  included,
  editable,
  disabled,
  episodeSelectionPreview,
  feedback,
  onToggle,
  onAdd,
}: {
  pick: Keyed;
  included: boolean;
  editable: boolean;
  disabled?: boolean;
  episodeSelectionPreview?: ProposalEditProps["episodeSelectionPreview"];
  feedback?: ReactNode;
  onToggle?: () => void;
  onAdd?: () => void;
}) => {
  const state = stateLabel(pick.kind, onAdd === undefined);
  const selection = episodeSelectionLabel(pick.item, episodeSelectionPreview);
  const season = seasonLabel(pick.item);

  return (
    <li
      className={cn(
        "flex items-start gap-3 border-border border-b px-1 py-3 last:border-b-0",
        !included && "opacity-55",
      )}
    >
      {editable && onToggle && pick.key !== "" && (
        <Checkbox
          className="mt-0.5 shrink-0"
          checked={included}
          disabled={disabled}
          aria-label={`Include ${pick.item.name}`}
          onChange={onToggle}
        />
      )}
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className={cn("font-medium text-sm", !included && "line-through")}>{pick.item.name}</span>
          {pick.item.year ? <span className="text-muted-foreground text-xs">{pick.item.year}</span> : null}
          {included ? (
            <Badge variant={state.variant}>{state.text}</Badge>
          ) : (
            <Badge variant="neutral">Not included</Badge>
          )}
          {season && <Badge variant="tune">{season}</Badge>}
          {selection && <Badge variant="suggest">{selection}</Badge>}
        </div>
        <p className="mt-1 text-muted-foreground text-xs">{itemDetails(pick.item)}</p>
        {included && pick.item.rationale && (
          <details className="mt-1.5 text-sm">
            <summary className="w-fit cursor-pointer text-muted-foreground text-xs hover:text-foreground">
              Why this title?
            </summary>
            <p className="mt-1 max-w-prose text-muted-foreground">
              {friendlyTitleRationale(pick.item.rationale)}
            </p>
          </details>
        )}
        {included && feedback && <div className="mt-2">{feedback}</div>}
      </div>
      {editable && onAdd && (
        <Button variant="outline" size="sm" disabled={disabled} onClick={onAdd}>
          <Plus aria-hidden /> Add <span className="sr-only">{pick.item.name}</span>
        </Button>
      )}
    </li>
  );
};

// The one pre-approval title composer used by both the first-channel review and Queue. Its
// state is only an ApprovalEdit delta; nothing persists until the existing approval gate runs.
const ProposalEdit = (props: ProposalEditProps) => {
  const {
    lineup,
    acquisitions,
    alternates = [],
    optionalSuggestions = [],
    movieCollections = [],
    movieCollectionsLoading = false,
    movieCollectionsIncomplete = false,
    onFindMore,
    findingMore = false,
    episodeSelectionPreview,
    value,
    onChange,
    renderFeedback,
    showNote = false,
    disabled,
    className,
  } = props;
  const controlled = Object.hasOwn(props, "value");
  const injectedMovieCollections = Object.hasOwn(props, "movieCollections");
  const [localEdit, setLocalEdit] = useState<ApprovalEditDTO | undefined>(value);
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");
  const editable = onChange !== undefined;
  const currentEdit = controlled ? value : localEdit;
  const dropped = currentEdit?.drop ?? [];
  const added = currentEdit?.add ?? [];
  const note = currentEdit?.note ?? "";

  const allPicks: Keyed[] = [
    ...lineup.map((item) => ({
      item,
      key: provisionKey(item),
      kind: item.inLibrary ? ("ready" as const) : ("missing" as const),
    })),
    ...acquisitions.map((item) => ({ item, key: provisionKey(item), kind: "missing" as const })),
  ];
  const optionalKeys = new Set(optionalSuggestions.map(provisionKey));
  const picks = allPicks.filter((pick) => !optionalKeys.has(pick.key));
  const addedKeys = new Set(added.map(provisionKey));
  const suggestionKeys = new Set<string>();
  const suggestions: Keyed[] = [
    ...optionalSuggestions.map((item) => ({
      item,
      key: provisionKey(item),
      kind: item.inLibrary ? ("ready" as const) : ("missing" as const),
    })),
    ...alternates.map((item) => ({
      item,
      key: provisionKey(item),
      kind: item.inLibrary ? ("ready" as const) : ("missing" as const),
    })),
  ].filter((pick) => {
    if (addedKeys.has(pick.key) || (!optionalKeys.has(pick.key) && dropped.includes(pick.key))) return false;
    if (suggestionKeys.has(pick.key)) return false;
    suggestionKeys.add(pick.key);
    return true;
  });

  // The visible review is the lookup boundary: collection suggestions must be
  // grounded in a movie the reviewer can already see, never in an unrelated
  // model guess. Current picks take precedence over optional suggestions when
  // the API's bounded request is full.
  const collectionKeys = movieCollectionKeys([
    ...lineup,
    ...acquisitions,
    ...added,
    ...optionalSuggestions,
    ...alternates,
  ]);
  const seenCollectionKeys = new Set(collectionKeys);
  const collectionResolution = searchApi.useResolveMovieCollections(
    { key: collectionKeys },
    {
      query: {
        enabled: editable && !injectedMovieCollections && collectionKeys.length > 0,
        retry: false,
        placeholderData: (previous) => previous,
      },
    },
  );
  const resolvedMovieCollections = (
    unwrap(collectionResolution.data, (body) => body)?.collections ?? []
  ).filter((collection) => collection.members.some((member) => seenCollectionKeys.has(provisionKey(member))));
  const displayedMovieCollections = injectedMovieCollections ? movieCollections : resolvedMovieCollections;
  const displayedMovieCollectionsLoading =
    movieCollectionsLoading ||
    (!injectedMovieCollections && collectionResolution.isFetching && collectionResolution.data === undefined);
  const displayedMovieCollectionsIncomplete =
    movieCollectionsIncomplete ||
    (!injectedMovieCollections && unwrap(collectionResolution.data, (body) => body)?.complete === false);

  const search = searchApi.useSearch(
    { q: query, scope: "all", limit: 8 },
    { query: { enabled: adding && query.trim().length > 1 } },
  );
  const candidates = unwrap(search.data, (body) => body.candidates) ?? [];

  const emit = (nextDropped: string[], nextAdded: ProposalItem[], nextNote: string) => {
    if (!onChange) return;
    if (nextDropped.length === 0 && nextAdded.length === 0 && nextNote === "") {
      if (!controlled) setLocalEdit(undefined);
      onChange(undefined);
      return;
    }
    const next = {
      ...(nextDropped.length > 0 ? { drop: nextDropped } : {}),
      ...(nextAdded.length > 0 ? { add: nextAdded } : {}),
      ...(nextNote !== "" ? { note: nextNote } : {}),
    };
    if (!controlled) setLocalEdit(next);
    onChange(next);
  };

  const toggleDrop = (key: string) => {
    const next = dropped.includes(key) ? dropped.filter((value) => value !== key) : [...dropped, key];
    emit(next, added, note);
  };

  const addCandidate = (candidate: SearchCandidate) => {
    const item = proposalItemFromCandidate(candidate);
    const next = [...added, item];
    emit(dropped, next, note);
    setQuery("");
    setAdding(false);
  };

  const removeAdded = (key: string) => {
    const next = added.filter((item) => provisionKey(item) !== key);
    emit(dropped, next, note);
  };

  const addSuggestion = (pick: Keyed) => {
    const nextDropped = dropped.includes(pick.key) ? dropped : [...dropped, pick.key];
    emit(nextDropped, [...added, pick.item], note);
  };

  const existingKeys = new Set([
    ...picks.map((pick) => pick.key),
    ...suggestions.map((pick) => pick.key),
    ...added.map(provisionKey),
  ]);
  const baseKeys = new Set(picks.map((pick) => pick.key));
  const selectedKeys = new Set([
    ...picks.filter((pick) => !dropped.includes(pick.key)).map((pick) => pick.key),
    ...added.map(provisionKey),
  ]);

  const addCollectionMembers = (members: SearchCandidate[]) => {
    let nextDropped = [...dropped];
    const nextAdded = [...added];
    const nextAddedKeys = new Set(nextAdded.map(provisionKey));
    for (const member of members) {
      const key = provisionKey(member);
      if (key === "") continue;
      if (baseKeys.has(key)) {
        nextDropped = nextDropped.filter((value) => value !== key);
        continue;
      }
      if (nextAddedKeys.has(key)) continue;
      if (suggestionKeys.has(key) && !nextDropped.includes(key)) nextDropped.push(key);
      nextAdded.push(proposalItemFromCandidate(member));
      nextAddedKeys.add(key);
    }
    emit(nextDropped, nextAdded, note);
  };
  const edited = dropped.length > 0 || added.length > 0 || note.trim() !== "";

  return (
    <div className={cn("flex min-w-0 flex-col gap-3", className)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 className="font-medium">Titles</h3>
        </div>
        {edited && editable && (
          <Button
            variant="ghost"
            size="sm"
            disabled={disabled}
            onClick={() => {
              if (!controlled) setLocalEdit(undefined);
              onChange(undefined);
            }}
          >
            <RotateCcw aria-hidden />
            Reset changes
          </Button>
        )}
      </div>

      <ul aria-label="Titles" className="rounded-md border border-border px-3">
        {picks.map((pick) => (
          <PickRow
            key={pick.key || `unkeyed-${pick.item.name}`}
            pick={pick}
            included={!dropped.includes(pick.key)}
            editable={editable}
            disabled={disabled}
            episodeSelectionPreview={episodeSelectionPreview}
            feedback={renderFeedback?.(pick.item)}
            onToggle={() => toggleDrop(pick.key)}
          />
        ))}
        {added.map((item) => {
          const key = provisionKey(item);
          return (
            <PickRow
              key={key}
              pick={{ item, key, kind: item.inLibrary ? "ready" : "missing" }}
              included
              editable={editable}
              disabled={disabled}
              episodeSelectionPreview={episodeSelectionPreview}
              feedback={renderFeedback?.(item)}
              onToggle={() => removeAdded(key)}
            />
          );
        })}
      </ul>

      <MovieCollectionChoices
        collections={displayedMovieCollections}
        selectedKeys={selectedKeys}
        editable={editable}
        disabled={disabled}
        loading={displayedMovieCollectionsLoading}
        incomplete={displayedMovieCollectionsIncomplete}
        onAddCollection={(collection) => addCollectionMembers(collection.members)}
        onAddMember={(member) => addCollectionMembers([member])}
      />

      {editable &&
        (adding ? (
          <div className="flex flex-col gap-2">
            <SearchCommand
              query={query}
              onQueryChange={setQuery}
              loading={search.isFetching}
              placeholder="Search for a movie or show…"
              onEscape={() => {
                setQuery("");
                setAdding(false);
              }}
              results={candidates
                .filter((candidate) => {
                  const key = provisionKey(candidate);
                  return key !== "" && !existingKeys.has(key);
                })
                .map((candidate) => ({
                  id: provisionKey(candidate),
                  scope: candidate.inLibrary ? ("library" as const) : ("tmdb" as const),
                  name: candidate.name,
                  ...(candidate.year ? { meta: String(candidate.year) } : {}),
                  inLibrary: candidate.inLibrary,
                }))}
              onSelect={(result) => {
                const candidate = candidates.find((value) => provisionKey(value) === result.id);
                if (candidate) addCandidate(candidate);
              }}
            />
            <Button
              variant="ghost"
              size="sm"
              className="self-start"
              onClick={() => {
                setQuery("");
                setAdding(false);
              }}
            >
              Cancel
            </Button>
          </div>
        ) : (
          <Button
            variant="outline"
            size="sm"
            className="self-start"
            disabled={disabled}
            onClick={() => setAdding(true)}
          >
            <Plus aria-hidden />
            Add title
          </Button>
        ))}

      {suggestions.length === 0 && onFindMore && (
        <Button
          variant="outline"
          size="sm"
          className="self-start"
          disabled={disabled || findingMore}
          onClick={onFindMore}
        >
          {findingMore && <LoaderCircle aria-hidden className="animate-spin" />}
          {findingMore ? "Finding more suggestions…" : "Find more suggestions"}
        </Button>
      )}

      {suggestions.length > 0 && (
        <details className="rounded-md border border-border px-3 py-2.5">
          <summary className="cursor-pointer text-sm">
            <span className="font-medium">More suggestions</span>
            <span className="ml-2 text-muted-foreground">
              {suggestions.length} {suggestions.length === 1 ? "option" : "options"}
            </span>
          </summary>
          <p className="mt-2 text-muted-foreground text-sm">Add any that belong on your channel.</p>
          <ul className="mt-2 border-border border-t">
            {suggestions.map((pick) => (
              <PickRow
                key={pick.key || `unkeyed-${pick.item.name}`}
                pick={pick}
                included
                editable={editable}
                disabled={disabled}
                episodeSelectionPreview={episodeSelectionPreview}
                feedback={renderFeedback?.(pick.item)}
                onAdd={() => addSuggestion(pick)}
              />
            ))}
          </ul>
          {onFindMore && (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2"
              disabled={disabled || findingMore}
              onClick={onFindMore}
            >
              {findingMore && <LoaderCircle aria-hidden className="animate-spin" />}
              {findingMore ? "Finding more suggestions…" : "Find more suggestions"}
            </Button>
          )}
        </details>
      )}

      {showNote && editable && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="approval-note">Note to the requester</Label>
          <Input
            id="approval-note"
            value={note}
            disabled={disabled}
            placeholder={edited ? "Explain what you changed" : "Optional"}
            onChange={(event) => {
              emit(dropped, added, event.target.value);
            }}
          />
        </div>
      )}
    </div>
  );
};

export { ProposalEdit };

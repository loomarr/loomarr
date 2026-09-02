import * as searchApi from "@loomarr/api/endpoints/search";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { SearchCandidate } from "@loomarr/api/models/searchCandidate";
import { unwrap } from "@loomarr/api/unwrap";
import { provisionKey } from "@loomarr/core/provision";
import { Plus, RotateCcw, X } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import { SearchCommand } from "../../shell";
import { episodeSelectionLabel } from "../episode-selection-label";
import type { ProposalEditProps } from "./proposal-edit.type";

// ProposalEdit — edit-before-approve (V25b, design.md §7.2 / D-K). An admin drops titles they
// don't want, adds ones the model missed, and leaves the requester a note explaining why their
// request came back altered.
//
// THE EDIT IS A DELTA, NOT A REPLACED LIST: `{drop: [key], add: [item], note}`. That is the
// backend's shape and it matters — `suggest.Approve` takes the edit as a PARAMETER so the
// decision about what gets acquired stays inside the one approval gate. Applying the edit
// client-side and posting a "final" list would move that decision outside the gate and leave
// auto-approve running different logic.
//
// Drops are keyed by PROVISIONING KEY, never by index or name. The Go comment on the field says
// why: an index means "the third one in the list I was looking at", which is wrong the moment
// anything reorders between render and submit. `provisionKey` mirrors Go's derivation exactly —
// a key that disagrees by one character does not error, it just matches nothing, and the title
// the admin removed gets acquired anyway.

// A pick with its derived key. An item with no usable id (no tmdbId, no tvdbId) yields "" and
// cannot be dropped — it also cannot have been enqueued, so there is nothing to remove.
type Keyed = { item: ProposalItem; key: string; kind: "lineup" | "acquire" };

const ProposalEdit = ({
  lineup,
  acquisitions,
  episodeSelectionPreview,
  onChange,
  renderFeedback,
  disabled,
  className,
}: ProposalEditProps) => {
  const [dropped, setDropped] = useState<string[]>([]);
  const [added, setAdded] = useState<ProposalItem[]>([]);
  const [note, setNote] = useState("");
  const [adding, setAdding] = useState(false);
  const [query, setQuery] = useState("");

  const picks: Keyed[] = [
    ...lineup.map((item) => ({ item, key: provisionKey(item), kind: "lineup" as const })),
    ...acquisitions.map((item) => ({ item, key: provisionKey(item), kind: "acquire" as const })),
  ];

  const search = searchApi.useSearch(
    { q: query, scope: "all", limit: 8 },
    { query: { enabled: adding && query.trim().length > 1 } },
  );
  const candidates = unwrap(search.data, (b) => b.candidates) ?? [];

  // Emit the edit in the API's shape, or `undefined` when nothing has been modified.
  //
  // ⚠ UNDEFINED, NOT AN EMPTY OBJECT. The handler maps a body with no drops, no adds and no
  // note to a nil edit precisely so an untouched approval is indistinguishable from the
  // pre-edit behaviour — same code path, untouched proposal bytes, empty ModSummary. Sending
  // `{drop: [], add: [], note: ""}` would still record "approved with modifications: none",
  // which is a different and false claim about what the admin did.
  const emit = (nextDropped: string[], nextAdded: ProposalItem[], nextNote: string) => {
    const trimmed = nextNote.trim();
    if (nextDropped.length === 0 && nextAdded.length === 0 && trimmed === "") {
      onChange(undefined);
      return;
    }
    onChange({
      ...(nextDropped.length > 0 ? { drop: nextDropped } : {}),
      ...(nextAdded.length > 0 ? { add: nextAdded } : {}),
      ...(trimmed !== "" ? { note: trimmed } : {}),
    });
  };

  const toggleDrop = (key: string) => {
    const next = dropped.includes(key) ? dropped.filter((k) => k !== key) : [...dropped, key];
    setDropped(next);
    emit(next, added, note);
  };

  const addCandidate = (c: SearchCandidate) => {
    // The candidate becomes a ProposalItem the backend enqueues like any acquisition. It is
    // never "in library" from here: an added title goes through the same idempotent enqueue as
    // anything the model proposed, and the provisioner decides what is already present.
    const item: ProposalItem = {
      name: c.name,
      mediaType: c.mediaType,
      inLibrary: false,
      ...(c.year ? { year: c.year } : {}),
      ...(c.tmdbId ? { tmdbId: c.tmdbId } : {}),
      ...(c.tvdbId ? { tvdbId: c.tvdbId } : {}),
    };
    const next = [...added, item];
    setAdded(next);
    emit(dropped, next, note);
    setQuery("");
    setAdding(false);
  };

  const removeAdded = (key: string) => {
    const next = added.filter((i) => provisionKey(i) !== key);
    setAdded(next);
    emit(dropped, next, note);
  };

  const existingKeys = new Set([...picks.map((p) => p.key), ...added.map(provisionKey)]);
  const edited = dropped.length > 0 || added.length > 0 || note.trim() !== "";

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-2">
          <h4 className="font-medium text-sm">What gets approved</h4>
          {edited && (
            <Button
              variant="ghost"
              size="sm"
              disabled={disabled}
              onClick={() => {
                setDropped([]);
                setAdded([]);
                setNote("");
                onChange(undefined);
              }}
            >
              <RotateCcw aria-hidden />
              Reset
            </Button>
          )}
        </div>

        <ul className="flex flex-col gap-1.5">
          {picks.map(({ item, key, kind }) => {
            const isDropped = key !== "" && dropped.includes(key);
            return (
              <li
                key={key || `unkeyed-${item.name}`}
                className={cn(
                  "flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm",
                  isDropped && "opacity-50",
                )}
              >
                <span className={cn("min-w-0 flex-1 truncate", isDropped && "line-through")}>
                  {item.name}
                  {item.year ? (
                    <span className="ml-2 font-mono text-static-400 text-xs">{item.year}</span>
                  ) : null}
                </span>
                <Badge variant={kind === "lineup" ? "lock" : "tune"}>
                  {kind === "lineup" ? "In library" : "Will acquire"}
                </Badge>
                {episodeSelectionLabel(item, episodeSelectionPreview) && (
                  <Badge variant="suggest">{episodeSelectionLabel(item, episodeSelectionPreview)}</Badge>
                )}
                {renderFeedback && !isDropped && renderFeedback(item)}
                {/* A pick with no usable id was never enqueueable, so there is nothing to drop
                    — the control is omitted rather than rendered inert. */}
                {key !== "" && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-7 shrink-0"
                    disabled={disabled}
                    aria-label={isDropped ? `Keep ${item.name}` : `Remove ${item.name}`}
                    onClick={() => toggleDrop(key)}
                  >
                    {isDropped ? <RotateCcw aria-hidden /> : <X aria-hidden />}
                  </Button>
                )}
              </li>
            );
          })}

          {added.map((item) => {
            const key = provisionKey(item);
            return (
              <li
                key={key}
                className="flex items-center gap-2 rounded-md border border-border bg-suggest-tint-15 px-3 py-2 text-sm"
              >
                <span className="min-w-0 flex-1 truncate">
                  {item.name}
                  {item.year ? (
                    <span className="ml-2 font-mono text-static-400 text-xs">{item.year}</span>
                  ) : null}
                </span>
                <Badge variant="suggest">Added</Badge>
                {episodeSelectionLabel(item, episodeSelectionPreview) && (
                  <Badge variant="suggest">{episodeSelectionLabel(item, episodeSelectionPreview)}</Badge>
                )}
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-7 shrink-0"
                  disabled={disabled}
                  aria-label={`Remove ${item.name}`}
                  onClick={() => removeAdded(key)}
                >
                  <X aria-hidden />
                </Button>
              </li>
            );
          })}
        </ul>

        {adding ? (
          <div className="flex flex-col gap-2">
            <SearchCommand
              query={query}
              onQueryChange={setQuery}
              loading={search.isFetching}
              // Escape does what Cancel does. Opt-in because the ⌘K palette binds Escape at
              // the window level and a default binding would close both (see onEscape).
              onEscape={() => {
                setQuery("");
                setAdding(false);
              }}
              // Already-picked titles and ones with no usable id are filtered out rather than
              // offered and then silently ignored by the backend.
              results={candidates
                .filter((c) => {
                  const k = provisionKey(c);
                  return k !== "" && !existingKeys.has(k);
                })
                .map((c) => ({
                  id: provisionKey(c),
                  scope: c.inLibrary ? ("library" as const) : ("tmdb" as const),
                  name: c.name,
                  ...(c.year ? { meta: String(c.year) } : {}),
                  inLibrary: c.inLibrary,
                }))}
              onSelect={(r) => {
                const c = candidates.find((x) => provisionKey(x) === r.id);
                if (c) addCandidate(c);
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
            Add a title…
          </Button>
        )}
      </div>

      {/* The note is the half of this feature that is about people rather than data: a request
          that comes back altered without explanation reads as arbitrary. Optional — approving
          unchanged needs no explanation (the same reasoning that kept Approve one click while
          Deny became two-step in V23). */}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="approval-note">Note to the requester</Label>
        <Input
          id="approval-note"
          value={note}
          disabled={disabled}
          placeholder={edited ? "Why did this come back changed?" : "Optional"}
          onChange={(e) => {
            setNote(e.target.value);
            emit(dropped, added, e.target.value);
          }}
        />
      </div>
    </div>
  );
};

export { ProposalEdit };

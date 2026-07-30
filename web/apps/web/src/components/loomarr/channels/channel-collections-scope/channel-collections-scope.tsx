import { ApiError, libraryApi } from "@loomarr/api";
import { Checkbox, Label } from "@/components/ui";
import { cn } from "@/lib";
import { FieldHelp } from "../../feedback";
import type { ChannelCollectionsScopeProps } from "./channel-collections-scope.type";

// ChannelCollectionsScope — `policy.scope.collections`, the "only what I shelved" narrowing
// (programming-design §2.2). The collections are the operator's own media-server BoxSets.
//
// ⚠ A CHECKBOX LIST, not a search box like ChannelSeriesScope one folder over. That difference
// is the shape of the data, not a style choice: series are an open corpus of thousands (so the
// only workable control is search), while collections are a small closed set the operator made
// by hand. Showing all of them is also the only way the empty state can say anything useful —
// a search box over zero collections just looks broken.

const ChannelCollectionsScope = ({ policy, onChange, className }: ChannelCollectionsScopeProps) => {
  const selected = policy.scope?.collections ?? [];
  const query = libraryApi.useListLibraryCollections();

  // 501 is "no library configured", which is a different thing from "no collections" and gets
  // no control at all — the Connections page is where that is fixed.
  //
  // ⚠ Read off the thrown ApiError, NOT `query.data.status`. A 501 is an error response, so it
  // never populates `data` — checking there leaves `unavailable` permanently false and the
  // component falls through to its EMPTY state, telling an operator with no media server to
  // "make a collection in your media server". Caught by the test, not by review; the same
  // detection ChannelIconField already uses for an unconfigured TMDB.
  const unavailable = query.error instanceof ApiError && query.error.status === 501;
  const collections = query.data?.status === 200 ? (query.data.data.collections ?? []) : [];

  const toggle = (id: string, on: boolean) => {
    const next = on ? [...selected, id] : selected.filter((c) => c !== id);
    // An emptied list must send [] (not undefined): `collections` is omitempty, so dropping the
    // key would leave the previous restriction in place and the field would appear to clear
    // while still filtering. Same trap as scope.series and runtimeMax.
    onChange({ ...policy, scope: { ...policy.scope, collections: next } });
  };

  if (unavailable) return null;

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <div className="flex items-center gap-1.5">
        <Label>Only these collections</Label>
        <FieldHelp label="Only these collections">
          Restrict the channel to titles in the collections you keep in your media server. Leave all unticked
          for no restriction. Changes to a collection are picked up the next time the channel rebuilds.
        </FieldHelp>
      </div>

      {query.isPending ? (
        <p className="text-muted-foreground text-sm">Loading your collections…</p>
      ) : query.isError ? (
        // Any other failure (the media server is down, a bad token) must NOT read as "you have
        // none" — that would send the operator to create a collection they may already have.
        <p className="text-muted-foreground text-sm">
          Couldn't load your collections. Check the media server connection in Settings.
        </p>
      ) : collections.length === 0 ? (
        // A real answer, not a failure: the operator has a library but has made no collections.
        // Saying so beats an empty box that reads as a broken control.
        <p className="text-muted-foreground text-sm">
          You have no collections yet. Make one in your media server and it will show up here.
        </p>
      ) : (
        <fieldset className="flex flex-col gap-2">
          <legend className="sr-only">Only these collections</legend>
          <div className="flex flex-wrap gap-x-5 gap-y-2">
            {collections.map((c) => (
              <div key={c.id} className="flex items-center gap-2">
                <Checkbox
                  id={`policy-collection-${c.id}`}
                  checked={selected.includes(c.id)}
                  onChange={(e) => toggle(c.id, e.target.checked)}
                />
                <Label htmlFor={`policy-collection-${c.id}`} className="font-normal">
                  {c.name}
                  {c.childCount ? (
                    <span className="ml-1.5 text-muted-foreground text-xs">{c.childCount}</span>
                  ) : null}
                </Label>
              </div>
            ))}
          </div>
        </fieldset>
      )}
    </div>
  );
};

export { ChannelCollectionsScope };

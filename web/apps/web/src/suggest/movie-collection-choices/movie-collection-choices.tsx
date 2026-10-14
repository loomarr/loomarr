import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { provisionKey } from "@loomarr/core/provision";
import { Check, LoaderCircle, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Disclosure } from "@/components/ui/disclosure";
import type { MovieCollectionChoicesProps } from "./movie-collection-choices.type";

const MAX_MOVIE_COLLECTION_KEYS = 24;

const movieCollectionKeys = (items: readonly ProposalItem[]) => {
  const keys: string[] = [];
  const seen = new Set<string>();
  for (const item of items) {
    if (item.mediaType !== "movie" || !Number.isInteger(item.tmdbId) || (item.tmdbId ?? 0) <= 0) continue;
    const key = provisionKey(item);
    if (!key.startsWith("movie:tmdb:") || seen.has(key)) continue;
    seen.add(key);
    keys.push(key);
    if (keys.length === MAX_MOVIE_COLLECTION_KEYS) break;
  }
  return keys;
};

const MovieCollectionChoices = ({
  collections,
  selectedKeys,
  editable,
  disabled,
  loading = false,
  incomplete = false,
  onAddCollection,
  onAddMember,
}: MovieCollectionChoicesProps) => {
  const actionableCollections = collections.filter((collection) =>
    collection.members.some((member) => {
      const key = provisionKey(member);
      return key !== "" && !selectedKeys.has(key);
    }),
  );
  if (actionableCollections.length === 0) {
    return loading ? (
      <p role="status" className="inline-flex items-center gap-2 text-muted-foreground text-sm">
        <LoaderCircle className="size-4 animate-spin" aria-hidden /> Checking for movie collections…
      </p>
    ) : null;
  }

  return (
    <section
      aria-label="Movie collections"
      className="flex flex-col gap-2 rounded-md border border-border p-3"
    >
      <div>
        <h3 className="font-medium text-sm">Complete a collection</h3>
        <p className="mt-0.5 text-muted-foreground text-xs">
          Keep the films together, or choose just the ones you want.
        </p>
      </div>

      {incomplete && (
        <p role="status" className="text-muted-foreground text-xs">
          Some collection details aren’t available right now.
        </p>
      )}

      {actionableCollections.map((collection) => {
        const members = collection.members.filter((member) => provisionKey(member) !== "");
        const selectedCount = members.filter((member) => selectedKeys.has(provisionKey(member))).length;
        const libraryCount = members.filter((member) => member.inLibrary).length;

        return (
          <div key={collection.tmdbId} className="rounded-md bg-static-900/50 px-3 py-2.5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0">
                <p className="truncate font-medium text-sm">{collection.name}</p>
                <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground text-xs">
                  <span>
                    {selectedCount} of {members.length} selected
                  </span>
                  <span aria-hidden>·</span>
                  <span>{libraryCount} in your library</span>
                </div>
              </div>
              {editable && (
                <Button
                  size="sm"
                  variant="outline"
                  aria-label={`Add all films from ${collection.name}`}
                  disabled={disabled}
                  onClick={() => onAddCollection(collection)}
                >
                  <Plus aria-hidden /> Add all films
                </Button>
              )}
            </div>

            <Disclosure>
              <Disclosure.SectionTrigger
                className="mt-1 py-1.5 text-sm"
                label={`Choose films from ${collection.name}`}
                title="Choose films"
              />
              <Disclosure.Panel className="pt-1">
                <ul className="divide-y divide-border border-border border-t">
                  {members.map((member) => {
                    const key = provisionKey(member);
                    const selected = selectedKeys.has(key);
                    return (
                      <li key={key} className="flex items-center gap-3 py-2.5">
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm">{member.name}</p>
                          <p className="mt-0.5 text-muted-foreground text-xs">
                            {[member.year, member.inLibrary ? "In your library" : "Not in your library"]
                              .filter(Boolean)
                              .join(" · ")}
                          </p>
                        </div>
                        {selected ? (
                          <span className="inline-flex items-center gap-1 text-muted-foreground text-xs">
                            <Check className="size-3.5" aria-hidden /> Selected
                          </span>
                        ) : (
                          editable && (
                            <Button
                              size="sm"
                              variant="ghost"
                              disabled={disabled}
                              onClick={() => onAddMember(member)}
                            >
                              <Plus aria-hidden /> Add <span className="sr-only">{member.name}</span>
                            </Button>
                          )
                        )}
                      </li>
                    );
                  })}
                </ul>
              </Disclosure.Panel>
            </Disclosure>
          </div>
        );
      })}
    </section>
  );
};

export { MAX_MOVIE_COLLECTION_KEYS, MovieCollectionChoices, movieCollectionKeys };

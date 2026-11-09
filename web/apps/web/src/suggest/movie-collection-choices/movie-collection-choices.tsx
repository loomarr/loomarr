import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { provisionKey } from "@loomarr/core/provision";
import { Action, Disclosure, Icon, icons, LoadingStrip } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import type { MovieCollectionChoicesProps } from "./movie-collection-choices.type";

// One per collection, so each list opens on its own. The collection's name rides in the
// description, which keeps every header's accessible name distinct.
const ChooseFilms = ({ collectionName, children }: { collectionName: string; children: ReactNode }) => {
  const [expanded, setExpanded] = useState(false);
  return (
    <Disclosure
      label="Choose films"
      description={`From ${collectionName}`}
      expanded={expanded}
      onExpandedChange={setExpanded}
    >
      {children}
    </Disclosure>
  );
};

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
      <p role="status" className="text-muted-foreground inline-flex items-center gap-2 text-sm">
        <LoadingStrip decorative width={32} /> Checking for movie collections…
      </p>
    ) : null;
  }

  return (
    <section
      aria-label="Movie collections"
      className="border-border flex flex-col gap-2 rounded-md border p-3"
    >
      <div>
        <h3 className="text-sm font-medium">Complete a collection</h3>
        <p className="text-muted-foreground mt-0.5 text-xs">
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
          <div key={collection.tmdbId} className="bg-static-900/50 rounded-md px-3 py-2.5">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{collection.name}</p>
                <div className="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                  <span>
                    {selectedCount} of {members.length} selected
                  </span>
                  <span aria-hidden>·</span>
                  <span>{libraryCount} in your library</span>
                </div>
              </div>
              {editable && (
                <Action
                  accessibilityLabel={`Add all films from ${collection.name}`}
                  disabled={disabled}
                  icon="plus"
                  onPress={() => onAddCollection(collection)}
                  variant="outline"
                >
                  Add all films
                </Action>
              )}
            </div>

            <ChooseFilms collectionName={collection.name}>
              <ul className="divide-border border-border divide-y border-t">
                {members.map((member) => {
                  const key = provisionKey(member);
                  const selected = selectedKeys.has(key);
                  return (
                    <li key={key} className="flex items-center gap-3 py-2.5">
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm">{member.name}</p>
                        <p className="text-muted-foreground mt-0.5 text-xs">
                          {[member.year, member.inLibrary ? "In your library" : "Not in your library"]
                            .filter(Boolean)
                            .join(" · ")}
                        </p>
                      </div>
                      {selected ? (
                        <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
                          <Icon decorative glyph={icons.selected} size="compact" tone="inherit" /> Selected
                        </span>
                      ) : (
                        editable && (
                          <Action
                            accessibilityLabel={`Add ${member.name}`}
                            disabled={disabled}
                            icon="plus"
                            onPress={() => onAddMember(member)}
                            variant="ghost"
                          >
                            Add
                          </Action>
                        )
                      )}
                    </li>
                  );
                })}
              </ul>
            </ChooseFilms>
          </div>
        );
      })}
    </section>
  );
};

export { MAX_MOVIE_COLLECTION_KEYS, MovieCollectionChoices, movieCollectionKeys };

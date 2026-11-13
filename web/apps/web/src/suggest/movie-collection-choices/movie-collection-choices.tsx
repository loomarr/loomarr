import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { provisionKey } from "@loomarr/core/provision";
import { Action, Disclosure, Icon, icons, LoadingStrip, Stack, Text } from "@loomarr/design-system";
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
      <div role="status">
        <Text as="p" className="inline-flex items-center gap-2" textRole="compact" tone="secondary">
          <LoadingStrip decorative width={32} /> Checking for movie collections…
        </Text>
      </div>
    ) : null;
  }

  return (
    <Stack
      aria-label="Movie collections"
      className="border-border rounded-md border p-3"
      gap="inline"
      render="section"
    >
      <div>
        <Text as="h3" textRole="cardLabel">
          Complete a collection
        </Text>
        <Text as="p" className="mt-0.5" textRole="hint" tone="secondary">
          Keep the films together, or choose just the ones you want.
        </Text>
      </div>

      {incomplete && (
        <div role="status">
          <Text as="p" textRole="hint" tone="secondary">
            Some collection details aren’t available right now.
          </Text>
        </div>
      )}

      {actionableCollections.map((collection) => {
        const members = collection.members.filter((member) => provisionKey(member) !== "");
        const selectedCount = members.filter((member) => selectedKeys.has(provisionKey(member))).length;
        const libraryCount = members.filter((member) => member.inLibrary).length;

        return (
          <div key={collection.tmdbId} className="bg-static-900/50 rounded-md px-3 py-2.5">
            <Stack align="center" direction="row" gap="inline" justify="between" wrap>
              <div className="min-w-0">
                <Text as="p" className="truncate" textRole="cardLabel">
                  {collection.name}
                </Text>
                <Stack align="center" columnGap="2" direction="row" marginTop={4} rowGap="1" wrap>
                  <Text as="span" textRole="hint" tone="secondary">
                    {selectedCount} of {members.length} selected
                  </Text>
                  <Text aria-hidden as="span" textRole="hint" tone="secondary">
                    ·
                  </Text>
                  <Text as="span" textRole="hint" tone="secondary">
                    {libraryCount} in your library
                  </Text>
                </Stack>
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
            </Stack>

            <ChooseFilms collectionName={collection.name}>
              <ul className="divide-border border-border divide-y border-t">
                {members.map((member) => {
                  const key = provisionKey(member);
                  const selected = selectedKeys.has(key);
                  return (
                    <Stack
                      align="center"
                      className="py-2.5"
                      direction="row"
                      gap="control"
                      key={key}
                      render="li"
                    >
                      <div className="min-w-0 flex-1">
                        <Text as="p" className="truncate" textRole="compact">
                          {member.name}
                        </Text>
                        <Text as="p" className="mt-0.5" textRole="hint" tone="secondary">
                          {[member.year, member.inLibrary ? "In your library" : "Not in your library"]
                            .filter(Boolean)
                            .join(" · ")}
                        </Text>
                      </div>
                      {selected ? (
                        <Text
                          as="span"
                          className="inline-flex items-center gap-1"
                          textRole="hint"
                          tone="secondary"
                        >
                          <Icon decorative glyph={icons.selected} size="compact" tone="inherit" /> Selected
                        </Text>
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
                    </Stack>
                  );
                })}
              </ul>
            </ChooseFilms>
          </div>
        );
      })}
    </Stack>
  );
};

export { MAX_MOVIE_COLLECTION_KEYS, MovieCollectionChoices, movieCollectionKeys };

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
        <Stack align="center" direction="row" gap="2" render="p">
          <LoadingStrip decorative width={32} />
          <Text as="span" textRole="compact" tone="secondary">
            Checking for movie collections…
          </Text>
        </Stack>
      </div>
    ) : null;
  }

  return (
    <Stack
      aria-label="Movie collections"
      borderColor="borderDecorative"
      borderRadius={8}
      borderWidth={1}
      gap="inline"
      padding={12}
      render="section"
    >
      <div>
        <Text as="h3" textRole="cardLabel">
          Complete a collection
        </Text>
        <Stack marginTop={2}>
          <Text as="p" textRole="hint" tone="secondary">
            Keep the films together, or choose just the ones you want.
          </Text>
        </Stack>
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
              <Stack flexShrink={1} minWidth={0}>
                <Text as="p" numberOfLines={1} textRole="cardLabel">
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
              </Stack>
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
              <Stack borderColor="borderDecorative" borderTopWidth={1} render="ul">
                {members.map((member, index) => {
                  const key = provisionKey(member);
                  const selected = selectedKeys.has(key);
                  return (
                    <Stack
                      align="center"
                      borderColor="borderDecorative"
                      borderTopWidth={index === 0 ? 0 : 1}
                      direction="row"
                      gap="control"
                      key={key}
                      paddingVertical={10}
                      render="li"
                    >
                      <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
                        <Text as="p" numberOfLines={1} textRole="compact">
                          {member.name}
                        </Text>
                        <Stack marginTop={2}>
                          <Text as="p" textRole="hint" tone="secondary">
                            {[member.year, member.inLibrary ? "In your library" : "Not in your library"]
                              .filter(Boolean)
                              .join(" · ")}
                          </Text>
                        </Stack>
                      </Stack>
                      {selected ? (
                        <Stack align="center" direction="row" gap="1" render="span">
                          <Icon decorative glyph={icons.selected} size="compact" tone="secondary" />
                          <Text as="span" textRole="hint" tone="secondary">
                            Selected
                          </Text>
                        </Stack>
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
              </Stack>
            </ChooseFilms>
          </div>
        );
      })}
    </Stack>
  );
};

export { MAX_MOVIE_COLLECTION_KEYS, MovieCollectionChoices, movieCollectionKeys };

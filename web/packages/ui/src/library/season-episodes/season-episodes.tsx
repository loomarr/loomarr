import { Icon, icons, PressableRow, Surface, Tabs, Text } from "@loomarr/design-system";
import { useEffect, useState } from "react";

import { FileFacts, lengthPhrase } from "../file-facts";
import { Line, Stack, Twirl } from "../library-poster";
import type {
  LibraryTitleEpisode,
  LibraryTitleFile,
  LibraryTitleFix,
  LibraryTitleSeason,
} from "../library-title/library-title.type";
import { useTitleLayout } from "../title-parts";

// SEASONS AND EPISODES (#4476): the seasons as underline tabs with the specials last, each episode
// with its still, a one-line summary and its length. For an admin a row opens to its own file.

const seasonLabel = (number: number) => (number === 0 ? "Specials" : `Season ${number}`);

// "3" or, for a file holding two, "3–4". An episode with no number (a special its show's catalog
// doesn't number) shows none.
const episodeNumber = (number: number, end?: number) => {
  if (number <= 0) {
    return "";
  }
  return end && end > number ? `${number}–${end}` : `${number}`;
};

// The episode's own file, asked for when its row opens: the series' answer holds no files.
const EpisodeFile = ({ id, load }: { id: string; load: NonNullable<LibraryTitleFix["onEpisodeFiles"]> }) => {
  const { wide, density } = useTitleLayout();
  const [files, setFiles] = useState<readonly LibraryTitleFile[]>();
  const [failed, setFailed] = useState(false);
  // The loader is a new function on every render of the page; the row asks once per episode.
  // biome-ignore lint/correctness/useExhaustiveDependencies: ask again only for another episode
  useEffect(() => {
    let live = true;
    load(id).then(
      (answer) => live && setFiles(answer),
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, [id]);
  return (
    <Surface borderWidth={0} gap="control" padding="control">
      <Text textRole="title">This episode's file</Text>
      {failed ? (
        <Text textRole="compact" tone="secondary">
          Couldn't load this file's details. Try again in a moment.
        </Text>
      ) : files === undefined ? (
        <Text textRole="compact" tone="secondary">
          Loading the file's details…
        </Text>
      ) : files.length === 0 ? (
        <Text textRole="compact" tone="secondary">
          No details for this file yet.
        </Text>
      ) : (
        files.map((file) => <FileFacts defaultMore={wide} density={density} file={file} key={file.path} />)
      )}
    </Surface>
  );
};

const EpisodeContent = ({ episode, open }: { episode: LibraryTitleEpisode; open?: boolean }) => {
  const { wide } = useTitleLayout();
  const number = episodeNumber(episode.number, episode.end);
  const name = episode.name || `Episode ${episode.number}`;
  const length = episode.lengthMillis ? lengthPhrase(episode.lengthMillis) : "";
  return (
    <Line alignItems="flex-start" flexWrap="nowrap" gap="control" width="100%">
      {/* With no still the frame stays, quiet, so the rows line up. */}
      <Surface
        aria-hidden
        backgroundColor="artworkPlaceholder"
        borderWidth={0}
        height={wide ? 90 : 63}
        overflow="hidden"
        width={wide ? 160 : 112}
      >
        {episode.still}
      </Surface>
      <Stack flexBasis={0} flexGrow={1} flexShrink={1} minWidth={0}>
        <Text numberOfLines={wide ? 1 : 2} textRole="body">
          {number ? `${number}. ${name}` : name}
        </Text>
        {episode.overview ? (
          <Text numberOfLines={wide ? 1 : 2} textRole="compact" tone="secondary">
            {episode.overview}
          </Text>
        ) : null}
        {episode.fromFile ? (
          <Text textRole="hint" tone="muted">
            Name from the file
          </Text>
        ) : null}
        {episode.ready ? null : (
          <Line flexWrap="nowrap">
            <Twirl />
            <Text textRole="compact" tone="secondary">
              Getting ready
            </Text>
          </Line>
        )}
        {!wide && length ? (
          <Text textRole="compact" tone="secondary">
            {length}
          </Text>
        ) : null}
      </Stack>
      {wide && length ? (
        <Text textRole="compact" tone="secondary">
          {length}
        </Text>
      ) : null}
      {open === undefined ? null : (
        <Stack alignSelf="center">
          <Icon decorative glyph={open ? icons.showLess : icons.showMore} size="compact" tone="muted" />
        </Stack>
      )}
    </Line>
  );
};

const SeasonEpisodes = ({
  seasons,
  onEpisodeFiles,
}: {
  seasons: readonly LibraryTitleSeason[];
  /** Absent for a member, whose rows don't open. */
  onEpisodeFiles?: LibraryTitleFix["onEpisodeFiles"];
}) => {
  const { density } = useTitleLayout();
  // The specials after the numbered seasons.
  const ordered = [...seasons].sort(
    (a, b) => (a.number || Number.MAX_SAFE_INTEGER) - (b.number || Number.MAX_SAFE_INTEGER),
  );
  // A season with episodes waiting on their order is the one to look at first.
  const first = ordered.find((s) => s.episodes.some((e) => e.fromFile)) ?? ordered[0];
  const [chosen, setChosen] = useState<number>();
  const [open, setOpen] = useState<string>();
  const current = ordered.find((s) => s.number === chosen) ?? first;
  if (!current) return null;
  return (
    <Stack aria-label="Seasons" gap="control" role="region">
      <Tabs
        appearance="underline"
        density={density}
        label="Seasons"
        onValueChange={(value) => {
          setChosen(Number(value));
          setOpen(undefined);
        }}
        options={ordered.map((s) => ({
          value: String(s.number),
          label: seasonLabel(s.number),
          count: String(s.episodes.length),
          countLabel: s.episodes.length === 1 ? "1 episode" : `${s.episodes.length} episodes`,
        }))}
        value={String(current.number)}
      />
      <Stack>
        {current.episodes.map((episode) => (
          <Stack borderBottomColor="borderDecorative" borderBottomWidth={1} key={episode.id}>
            {onEpisodeFiles ? (
              <PressableRow
                aria-expanded={open === episode.id}
                onPress={() => setOpen(open === episode.id ? undefined : episode.id)}
              >
                <EpisodeContent episode={episode} open={open === episode.id} />
              </PressableRow>
            ) : (
              <Stack paddingVertical="inline">
                <EpisodeContent episode={episode} />
              </Stack>
            )}
            {onEpisodeFiles && open === episode.id ? (
              <EpisodeFile id={episode.id} load={onEpisodeFiles} />
            ) : null}
          </Stack>
        ))}
      </Stack>
    </Stack>
  );
};

export { SeasonEpisodes };

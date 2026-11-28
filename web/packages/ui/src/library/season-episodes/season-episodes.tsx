import { Icon, icons, PressableRow, Surface, Tabs, Text, VisuallyHidden } from "@loomarr/design-system";
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
const EpisodeFile = ({
  id,
  load,
  readAt,
  onSay,
}: {
  id: string;
  load: NonNullable<LibraryTitleFix["onEpisodeFiles"]>;
  readAt?: number;
  /** What the panel is waiting on, or couldn't do; "" once its files are shown. */
  onSay: (words: string) => void;
}) => {
  const { wide, density } = useTitleLayout();
  const [files, setFiles] = useState<readonly LibraryTitleFile[]>();
  const [failed, setFailed] = useState(false);
  // The loader is a new function on every render of the page; the row asks again only for another
  // episode, or when the title has been read again. What it shows stays up meanwhile.
  // biome-ignore lint/correctness/useExhaustiveDependencies: the loader's identity is not a reason
  useEffect(() => {
    let live = true;
    load(id).then(
      (answer) => {
        if (!live) return;
        setFiles(answer);
        setFailed(false);
      },
      () => live && setFailed(true),
    );
    return () => {
      live = false;
    };
  }, [id, readAt]);
  const words =
    files === undefined
      ? failed
        ? "Couldn't load this file's details. Try again in a moment."
        : "Loading the file's details…"
      : files.length === 0
        ? "No details for this file yet."
        : "";
  // biome-ignore lint/correctness/useExhaustiveDependencies: say it when the words change
  useEffect(() => {
    onSay(words);
    return () => onSay("");
  }, [words]);
  return (
    <Surface borderWidth={0} gap="control" padding="control">
      <Text textRole="body">This episode's file</Text>
      {/* The seasons' live region reads these words out, so this copy is for the eye. */}
      {words ? (
        <Text aria-hidden textRole="compact" tone="secondary">
          {words}
        </Text>
      ) : null}
      {files?.map((file) => (
        <FileFacts defaultMore={wide} density={density} file={file} key={file.path} />
      ))}
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
        {/* A button element centres its text, so each line says where it starts. */}
        <Text numberOfLines={wide ? 1 : 2} textAlign="left" textRole="body">
          {number ? `${number}. ${name}` : name}
        </Text>
        {episode.overview ? (
          <Text numberOfLines={wide ? 1 : 2} textAlign="left" textRole="compact" tone="secondary">
            {episode.overview}
          </Text>
        ) : null}
        {episode.fromFile ? (
          <Text textAlign="left" textRole="hint" tone="muted">
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
          <Text textAlign="left" textRole="compact" tone="secondary">
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
  readAt,
}: {
  seasons: readonly LibraryTitleSeason[];
  readAt?: number;
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
  const [said, setSaid] = useState("");
  if (!current) return null;
  return (
    <Stack aria-label="Seasons" gap="control" role="region">
      <VisuallyHidden as="h2">
        <Text textRole="headline">Seasons</Text>
      </VisuallyHidden>
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
              <EpisodeFile id={episode.id} load={onEpisodeFiles} onSay={setSaid} readAt={readAt} />
            ) : null}
          </Stack>
        ))}
      </Stack>
      {/* Mounted before it has anything to say: a live region added with its words is often missed. */}
      {onEpisodeFiles ? (
        <VisuallyHidden>
          <Text aria-live="polite" textRole="compact">
            {said}
          </Text>
        </VisuallyHidden>
      ) : null}
    </Stack>
  );
};

export { SeasonEpisodes };

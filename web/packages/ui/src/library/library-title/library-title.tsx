import { formatBytes } from "@loomarr/core/format";
import { Action, Badge, Disclosure, StatusDot, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import { useState } from "react";
import { ChannelIdentity } from "../../identity";
import { LibraryPoster, Line, PAGE_GUTTER, Stack, Twirl } from "../library-poster";
import { PickTitle } from "../pick-title";
import { lengthPhrase, TechnicalDetails } from "../technical-details";
import type {
  LibraryTitleDetail,
  LibraryTitleFile,
  LibraryTitleProps,
  LibraryTitleSeason,
} from "./library-title.type";

// A TITLE'S PAGE (#2151 decisions 25–26, design/library variant D): big artwork, the facts in plain
// words, the story, the seasons, and the file behind one "Technical details" disclosure.
// Presentational: the app owns the request and draws the artwork.

// The poster sits beside the story from a tablet up, above it on a phone, as in the mock.
const SIDE_BY_SIDE = 768;

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// "1999 · 7 seasons" or "1996 · 1 h 38 min": two facts at most.
const factLine = (title: LibraryTitleDetail) => {
  const seasons = title.seasons.filter((s) => s.number > 0).length;
  const second =
    title.kind === "series"
      ? seasons > 0
        ? plural(seasons, "season", "seasons")
        : ""
      : title.runtimeMillis
        ? lengthPhrase(title.runtimeMillis)
        : "";
  return [title.year ? String(title.year) : "", second].filter(Boolean).join(" · ");
};

const Heading = ({ children }: { children: string }) => (
  <Text as="h2" textRole="headline">
    {children}
  </Text>
);

// "3" or, for a file holding two, "3–4".
const episodeNumber = (number: number, end?: number) =>
  end && end > number ? `${number}–${end}` : `${number}`;

const Season = ({ season }: { season: LibraryTitleSeason }) => {
  const [open, setOpen] = useState(false);
  const label = season.number === 0 ? "Specials" : `Season ${season.number}`;
  return (
    <Disclosure
      description={plural(season.episodes.length, "episode", "episodes")}
      expanded={open}
      label={label}
      onExpandedChange={setOpen}
    >
      <Stack gap="inline">
        {season.episodes.map((episode) => (
          <Line
            alignItems="baseline"
            borderBottomColor="borderDecorative"
            borderBottomWidth={1}
            flexWrap="nowrap"
            gap="control"
            key={episode.id}
            paddingVertical="inline"
          >
            <Stack width={32}>
              <Text textRole="compact" tone="muted">
                {episodeNumber(episode.number, episode.end)}
              </Text>
            </Stack>
            <Stack flexBasis={0} flexGrow={1} minWidth={0}>
              <Text textRole="body">{episode.name || `Episode ${episode.number}`}</Text>
              {episode.fromFile ? (
                <Text textRole="hint" tone="muted">
                  Name from the file
                </Text>
              ) : null}
            </Stack>
            {episode.ready ? null : (
              <Line alignSelf="center" flexWrap="nowrap">
                <Twirl />
                <Text textRole="compact" tone="secondary">
                  Getting ready
                </Text>
              </Line>
            )}
          </Line>
        ))}
      </Stack>
    </Disclosure>
  );
};

// "Wrong title?" (decision 26) follows what decides: the app's own page, a note when the file
// itself says, else a search, and Forget my answer when the household answered.
const WrongTitle = ({
  title,
  onOpenApp,
  onSearch,
  onLink,
  onForget,
}: Pick<LibraryTitleProps, "title" | "onOpenApp" | "onSearch" | "onLink" | "onForget">) => {
  const [done, setDone] = useState<string>();
  const [failed, setFailed] = useState(false);
  const { by, app } = title.match;
  if (by === "app" && app) {
    return (
      <Surface gap="inline" padding="control">
        <Text textRole="body">{`${app} decides what this is. Change it in ${app}.`}</Text>
        {onOpenApp ? (
          <Line>
            <Action onPress={onOpenApp} variant="outline">
              {`Open ${app}`}
            </Action>
          </Line>
        ) : null}
      </Surface>
    );
  }
  if (by === "file") {
    return (
      <Surface gap="inline" padding="control">
        <Text textRole="body">
          This file's name says what it is. Rename it, or change its NFO, to fix it.
        </Text>
      </Surface>
    );
  }
  if (done) {
    return (
      <Line>
        <StatusDot label="Done" tone="ok" />
        <Text textRole="body">{`Done — ${done} is sorted.`}</Text>
      </Line>
    );
  }
  const answer = (save: Promise<void>, name?: string) => {
    setFailed(false);
    save.then(
      () => name && setDone(name),
      () => setFailed(true),
    );
  };
  return (
    <Stack gap="inline">
      <PickTitle
        kind={title.kind}
        onPick={(c) => answer(onLink(c.externalId), c.name)}
        onSearch={onSearch}
        others={[]}
      />
      {failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't save your answer. Try again in a moment.
        </Text>
      ) : null}
      {by === "you" ? (
        <Line>
          <Action onPress={() => answer(onForget())} variant="ghost">
            Forget my answer
          </Action>
        </Line>
      ) : null}
    </Stack>
  );
};

type Busy = "update" | "check";

// Update details · Check files again · Wrong title?, as the mock words them. A press waits for its
// answer; a check that went through says so, and one that failed says to try again.
const TitleActions = (props: Omit<LibraryTitleProps, "onBack">) => {
  const { onUpdateDetails, onCheckFiles } = props;
  const [busy, setBusy] = useState<Busy>();
  const [checked, setChecked] = useState(false);
  const [failed, setFailed] = useState<Busy>();
  const [wrong, setWrong] = useState(false);
  // A press while one is out is dropped, rather than disabling the buttons: a disabled button
  // loses the keyboard focus it holds.
  const run = (which: Busy, act: () => Promise<void>) => {
    if (busy) return;
    setBusy(which);
    setFailed(undefined);
    act().then(
      () => {
        setBusy(undefined);
        if (which === "check") setChecked(true);
      },
      () => {
        setBusy(undefined);
        setFailed(which);
      },
    );
  };
  return (
    <Stack gap="control">
      <Line>
        <Action onPress={() => run("update", onUpdateDetails)} variant="outline">
          {busy === "update" ? "Updating…" : "Update details"}
        </Action>
        <Action onPress={() => run("check", onCheckFiles)} variant="outline">
          {busy === "check" ? "Checking…" : checked ? "Checked just now" : "Check files again"}
        </Action>
        <Action onPress={() => setWrong(!wrong)} variant="ghost">
          Wrong title?
        </Action>
      </Line>
      {failed ? (
        <Text textRole="body" tone="secondary">
          {failed === "update"
            ? "Couldn't update the details just now. Try again in a moment."
            : "Couldn't check the files just now. Try again in a moment."}
        </Text>
      ) : null}
      {wrong ? <WrongTitle {...props} /> : null}
    </Stack>
  );
};

// An extra copy (decision 37) is a calm warning with one primary action. The card stays until the
// page refetches without the file; a press while one is out is dropped.
const ExtraCopy = ({
  file,
  onRemove,
}: {
  file: LibraryTitleFile;
  onRemove: (path: string) => Promise<void>;
}) => {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const remove = () => {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    onRemove(file.path).then(
      () => setBusy(false),
      () => {
        setBusy(false);
        setFailed(true);
      },
    );
  };
  return (
    <Surface borderColor="stateWarning" gap="control" padding="section">
      <Text as="h2" textRole="title">{`An extra copy is taking ${formatBytes(file.size)}`}</Text>
      <Text textRole="body" tone="secondary">
        Loomarr plays the better copy, so this one never plays. Removing it moves it to Loomarr's trash in the
        same folder, which empties after 30 days.
      </Text>
      {failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't remove it. Loomarr may not be allowed to change this folder.
        </Text>
      ) : null}
      <Line>
        <Action onPress={remove} tone="primary">
          Remove extra copy
        </Action>
      </Line>
    </Surface>
  );
};

// A file that can't be played (decision 24, the word sheet's "1 episode can't be played — the file
// may be damaged" + "Check again"): a calm warning whose one action reads the files again.
const Unplayable = ({
  kind,
  count,
  onCheck,
}: {
  kind: LibraryTitleDetail["kind"];
  count: number;
  onCheck: () => Promise<void>;
}) => {
  const [busy, setBusy] = useState(false);
  const [checked, setChecked] = useState(false);
  const [failed, setFailed] = useState(false);
  const check = () => {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    onCheck().then(
      () => {
        setBusy(false);
        setChecked(true);
      },
      () => {
        setBusy(false);
        setFailed(true);
      },
    );
  };
  const what =
    kind === "movie"
      ? "This movie can't be played — the file may be damaged"
      : count === 1
        ? "1 episode can't be played — the file may be damaged"
        : `${count} episodes can't be played — the files may be damaged`;
  return (
    <Surface borderColor="stateWarning" gap="control" padding="section">
      <Text as="h2" textRole="title">
        {what}
      </Text>
      {failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't check the files just now. Try again in a moment.
        </Text>
      ) : null}
      <Line>
        <Action onPress={check} tone="primary">
          {checked ? "Checked just now" : "Check again"}
        </Action>
      </Line>
    </Surface>
  );
};

const LibraryTitle = ({ onBack, ...actions }: LibraryTitleProps) => {
  const { title } = actions;
  const wide = useWindowWidth() >= SIDE_BY_SIDE;
  const facts = factLine(title);
  return (
    <Stack
      alignSelf="center"
      gap="screen"
      maxWidth={1152 + 2 * PAGE_GUTTER.wide}
      padding={wide ? PAGE_GUTTER.wide : PAGE_GUTTER.phone}
      width="100%"
    >
      <Line>
        <Action icon="back" onPress={onBack} variant="ghost">
          Library
        </Action>
      </Line>

      <Stack
        alignItems={wide ? "center" : "stretch"}
        flexDirection={wide ? "row" : "column"}
        gap={wide ? 40 : "section"}
      >
        <Stack alignSelf="center" width={wide ? 208 : 176}>
          <LibraryPoster art={title.poster} big name={title.name} />
        </Stack>
        <Stack flexBasis={wide ? 0 : undefined} flexGrow={1} gap="control" minWidth={0}>
          {/* A name no one has fixed yet can be a file name with no spaces; it breaks mid-word. */}
          <Text as="h1" style={{ overflowWrap: "anywhere" }} textRole="display">
            {title.name}
          </Text>
          {facts || title.rating ? (
            <Line gap="control">
              {facts ? (
                <Text textRole="body" tone="secondary">
                  {facts}
                </Text>
              ) : null}
              {title.rating ? <Badge tone="neutral">{title.rating}</Badge> : null}
            </Line>
          ) : null}
          {title.genres.length > 0 ? (
            <Text textRole="compact" tone="muted">
              {title.genres.join(", ")}
            </Text>
          ) : null}
          {title.overview ? <Text textRole="body">{title.overview}</Text> : null}
          <TitleActions {...actions} />
        </Stack>
      </Stack>

      {title.unplayable > 0 ? (
        <Unplayable count={title.unplayable} kind={title.kind} onCheck={actions.onCheckFiles} />
      ) : null}

      {title.files
        .filter((file) => file.extra)
        .map((file) => (
          <ExtraCopy file={file} key={file.path} onRemove={actions.onRemoveExtra} />
        ))}

      {actions.onChooseOrder && title.seasons.some((s) => s.episodes.some((e) => e.fromFile)) ? (
        <Surface
          backgroundColor="stateWarningSurface"
          borderWidth={0}
          paddingHorizontal={14}
          paddingVertical={10}
        >
          <Line columnGap="control" flexWrap="wrap" rowGap="inline">
            <Line flexBasis={240} flexGrow={1} flexWrap="nowrap" minWidth={0}>
              <StatusDot label="Needs you" tone="warn" />
              <Text textRole="compact">We're not sure which order these episodes are in.</Text>
            </Line>
            <Action onPress={actions.onChooseOrder} variant="outline">
              Choose
            </Action>
          </Line>
        </Surface>
      ) : null}

      {title.channels.length > 0 ? (
        <Stack aria-label="On these channels" gap="control" role="region">
          <Heading>On these channels</Heading>
          <Line columnGap="section" rowGap="control">
            {title.channels.map((c) => (
              <ChannelIdentity
                channel={{
                  channelLogoState: "missing",
                  channelName: c.name,
                  channelNumber: String(c.number),
                }}
                key={c.id}
              />
            ))}
          </Line>
        </Stack>
      ) : null}

      {title.seasons.length > 0 ? (
        <Stack aria-label="Seasons" gap="control" role="region">
          <Heading>Seasons</Heading>
          <Stack gap="inline">
            {/* The specials after the numbered seasons. */}
            {[...title.seasons]
              .sort((a, b) => (a.number || Number.MAX_SAFE_INTEGER) - (b.number || Number.MAX_SAFE_INTEGER))
              .map((season) => (
                <Season key={season.number} season={season} />
              ))}
          </Stack>
        </Stack>
      ) : null}

      <TechnicalDetails title={title} />
    </Stack>
  );
};

export { LibraryTitle };

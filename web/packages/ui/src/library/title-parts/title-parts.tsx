import { Action, Surface, Text, useWindowWidth, VisuallyHidden } from "@loomarr/design-system";
import { cloneElement, type ReactNode, useState } from "react";
import { ChannelIdentity } from "../../identity";
import { Line, Stack } from "../library-poster";
import type {
  LibraryPersonLink,
  LibraryTitleAiring,
  LibraryTitleDetail,
  LibraryTitleFix,
  LibraryTitlePerson,
  LibraryTitlePlaying,
  LibraryTitleProps,
} from "../library-title/library-title.type";
import { lengthPhrase, picturePhrase, soundPhrase } from "../technical-details";

// The parts of the TV-app title page (#4476): the hero, the channels and the people.

// The hero grows and the page's gutter widens from a tablet up, as in the mock.
const SIDE_BY_SIDE = 768;

// A phone's actions take the touch target.
const useTitleLayout = () => {
  const wide = useWindowWidth() >= SIDE_BY_SIDE;
  return { wide, density: wide ? ("pointer" as const) : ("touch" as const) };
};

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// Year, then length for a movie or the season count for a series.
const heroFacts = (title: LibraryTitleDetail) => {
  const seasons = title.seasons.filter((s) => s.number > 0).length;
  const second =
    title.kind === "series"
      ? seasons > 0
        ? plural(seasons, "season", "seasons")
        : ""
      : title.runtimeMillis
        ? lengthPhrase(title.runtimeMillis)
        : "";
  return [title.year ? String(title.year) : "", second].filter(Boolean);
};

// A member's one technical line: "1080p HD · Stereo · English subtitles".
const playingLine = ({ picture, audioChannels, subtitleLanguages }: LibraryTitlePlaying) =>
  [
    picture ? picturePhrase(picture) : "",
    audioChannels ? soundPhrase(audioChannels) : "",
    subtitleLanguages.length > 0 ? `${subtitleLanguages.join(", ")} subtitles` : "",
  ]
    .filter(Boolean)
    .join(" · ");

// The separator's column, which sits left of its fact.
const DOT = 20;

// Facts with a dot BETWEEN them, however the line wraps: each fact carries its dot on its left,
// and the row is pulled one dot-width outward and clipped, so the dot that would start a line is
// out of sight and no line ends with one.
const Facts = ({ facts }: { facts: readonly (ReactNode | false)[] }) => (
  <Stack overflow="hidden">
    <Line columnGap={0} marginLeft={-DOT}>
      {facts.filter(Boolean).map((fact, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: a fixed list of facts that never reorders
        <Line flexShrink={1} flexWrap="nowrap" gap={0} key={i} minWidth={0}>
          <Stack aria-hidden alignItems="center" width={DOT}>
            <Text textRole="compact" tone="muted">
              ·
            </Text>
          </Stack>
          <Stack flexShrink={1} minWidth={0}>
            {fact}
          </Stack>
        </Line>
      ))}
    </Line>
  </Stack>
);

// The backdrop with the title over its foot. The fade to the page keeps the title readable over any
// picture; with no backdrop the placeholder surface stands in. `actions` sit over its top, each on
// its own solid surface.
const Hero = ({ title, actions }: { title: LibraryTitleDetail; actions: ReactNode }) => {
  const { wide } = useTitleLayout();
  return (
    // The title's words are inside the picture and its fade, so a longer title grows the hero.
    <Surface
      backgroundColor="artworkPlaceholder"
      borderWidth={0}
      gap="section"
      justifyContent="space-between"
      minHeight={wide ? 420 : 300}
      overflow="hidden"
      position="relative"
    >
      <Stack bottom={0} left={0} position="absolute" right={0} top={0}>
        {title.backdrop}
      </Stack>
      <Stack
        aria-hidden
        bottom={0}
        height="80%"
        left={0}
        pointerEvents="none"
        position="absolute"
        right={0}
        style={{ backgroundImage: "linear-gradient(transparent, var(--surfaceCanvas) 80%)" }}
      />
      <Stack flexDirection="row" justifyContent="space-between" padding="control" position="relative">
        {actions}
      </Stack>
      <Stack gap="inline" padding="control" position="relative">
        {title.logo ? (
          <>
            {/* The logo stands for the name on screen; the heading still says it. */}
            <VisuallyHidden as="h1">
              <Text textRole="display">{title.name}</Text>
            </VisuallyHidden>
            <Stack aria-hidden height={wide ? 96 : 64} maxWidth={wide ? 420 : 280} width="100%">
              {title.logo}
            </Stack>
          </>
        ) : (
          // A name no one has fixed yet can be a file name with no spaces; it breaks mid-word.
          <Text as="h1" style={{ overflowWrap: "anywhere" }} textRole="display">
            {title.name}
          </Text>
        )}
        <Facts
          facts={[
            ...heroFacts(title).map((fact) => (
              <Text key={fact} textRole="body" tone="secondary">
                {fact}
              </Text>
            )),
            // A bordered chip in the body face: mono is for file names only.
            Boolean(title.rating) && (
              <Surface backgroundColor="transparent" paddingHorizontal="inline">
                <Text textRole="compact">{title.rating}</Text>
              </Surface>
            ),
            title.genres.length > 0 && (
              <Text textRole="compact" tone="muted">
                {title.genres.join(", ")}
              </Text>
            ),
            Boolean(title.score) && <Text textRole="body">{`Score ${title.score?.toFixed(1)}/10`}</Text>,
          ]}
        />
        {title.tagline ? (
          <Text style={{ fontStyle: "italic" }} textRole="body">
            {title.tagline}
          </Text>
        ) : null}
      </Stack>
    </Surface>
  );
};

type ChannelRowData = LibraryTitleAiring | Pick<LibraryTitleAiring, "channel">;

// One channel and when it shows the title. Where the app can name its player, the whole row is that
// link: it keeps its own semantics and navigation and takes only the row's layout and focus. The
// focus look is the row's amber border; forced colours turn the transparent outline visible.
const ChannelRow = ({ row, link }: { row: ChannelRowData; link: ReturnType<NonNullable<WatchLink>> }) => {
  const [focused, setFocused] = useState(false);
  const { channel } = row;
  const card = (
    <Surface
      borderColor={focused ? "actionFocus" : undefined}
      borderWidth={link ? 2 : undefined}
      gap="inline"
      padding="control"
    >
      <ChannelIdentity
        channel={{
          channelLogoState: channel.logo ? "ready" : "missing",
          channelName: channel.name,
          channelNumber: String(channel.number),
        }}
        logo={channel.logo}
      />
      {"when" in row ? (
        <Stack>
          <Text textRole="body">{row.when}</Text>
          {row.episode ? (
            <Text textRole="compact" tone="secondary">
              {row.episode}
            </Text>
          ) : null}
          {row.forecast ? (
            <Text textRole="compact" tone="muted">
              Expected time, it can still change
            </Text>
          ) : null}
        </Stack>
      ) : (
        <Text textRole="compact" tone="muted">
          Nothing scheduled soon
        </Text>
      )}
    </Surface>
  );
  if (!link) return card;
  return cloneElement(
    link,
    {
      onBlur: () => setFocused(false),
      onFocus: () => setFocused(true),
      style: {
        color: "inherit",
        display: "block",
        outline: focused ? "2px solid transparent" : "none",
        textDecoration: "none",
      },
    },
    card,
  );
};

type WatchLink = LibraryTitleProps["watchLink"];

// The page's main content: when each channel shows it next, earliest first, then the channels
// that hold it with nothing scheduled.
const OnChannels = ({
  title,
  watchLink,
  collectionsLink,
}: {
  title: LibraryTitleDetail;
  watchLink?: WatchLink;
  collectionsLink?: LibraryTitleFix["collectionsLink"];
}) => {
  const { density } = useTitleLayout();
  const airings = title.airings ?? [];
  const scheduled = new Set(airings.map((a) => a.channel.id));
  // An airing names its channel by id, so the title's own channel entry is the one drawn.
  const channels = new Map(title.channels.map((c) => [c.id, c]));
  const rows: ChannelRowData[] = [
    ...airings.map((a) => ({ ...a, channel: channels.get(a.channel.id) ?? a.channel })),
    ...title.channels.filter((c) => !scheduled.has(c.id)).map((channel) => ({ channel })),
  ];
  // The region is named by what it says, so a title on no channel has no "On these channels".
  const heading = rows.length > 0 ? "On these channels" : "Not on a channel yet";
  return (
    <Stack aria-label={heading} gap="control" role="region">
      <Text as="h2" textRole="headline">
        {heading}
      </Text>
      {rows.length === 0 ? (
        <>
          <Text textRole="body" tone="secondary">
            A channel can play it once it's in one of the channel's collections.
          </Text>
          {collectionsLink ? (
            <Line>
              <Action accessibilityRole="link" density={density} render={collectionsLink} variant="outline">
                Add it to a collection
              </Action>
            </Line>
          ) : null}
        </>
      ) : null}
      {rows.map((row) => (
        <ChannelRow key={row.channel.id} link={watchLink?.(row.channel.id)} row={row} />
      ))}
    </Stack>
  );
};

const initials = (name: string) => {
  const words = name.split(/\s+/).filter(Boolean);
  return [words[0], words.length > 1 ? words[words.length - 1] : undefined]
    .map((word) => word?.[0]?.toUpperCase() ?? "")
    .join("");
};

const CREW = [
  ["creator", "Created by"],
  ["director", "Directed by"],
  ["writer", "Written by"],
] as const;

// A credit's name: a link to the person's titles when the app can name them, else plain text.
const PersonName = ({
  person,
  personLink,
  center = false,
}: {
  person: LibraryTitlePerson;
  personLink?: LibraryPersonLink;
  center?: boolean;
}) => {
  const { density } = useTitleLayout();
  return personLink && person.personId ? (
    <Action accessibilityRole="link" density={density} render={personLink(person.personId)} variant="ghost">
      {person.name}
    </Action>
  ) : (
    <Text textAlign={center ? "center" : undefined} textRole={center ? "label" : "compact"}>
      {person.name}
    </Text>
  );
};

// The crew and the title's other facts as labelled lines, then the cast with their initials.
const CastAndCrew = ({
  title,
  personLink,
}: {
  title: LibraryTitleDetail;
  personLink?: LibraryPersonLink;
}) => {
  const credits: readonly LibraryTitlePerson[] = title.credits ?? [];
  const cast = credits.filter((c) => c.role === "actor");
  const crew = CREW.flatMap(([role, label]) => {
    const people = credits.filter((c) => c.role === role);
    if (people.length === 0) return [];
    const linked = personLink && people.some((p) => p.personId);
    const value = linked ? (
      <Line alignItems="center" columnGap="control" key={label} rowGap="inline">
        {people.map((p) => (
          <PersonName key={`${p.name}:${p.personId}`} person={p} personLink={personLink} />
        ))}
      </Line>
    ) : (
      <Text key={label} textRole="compact">
        {people.map((p) => p.name).join(", ")}
      </Text>
    );
    return [{ label, value }];
  });
  const rows = [
    ...crew,
    ...(title.about ?? [])
      .filter((row) => row.value)
      .map((row) => ({
        label: row.label,
        value: (
          <Text key={row.label} textRole="compact">
            {row.value}
          </Text>
        ),
      })),
  ];
  return (
    <>
      {rows.length > 0 ? (
        <Stack aria-label="About" gap="inline" role="region">
          {rows.map((row) => (
            <Line alignItems="baseline" flexWrap="nowrap" gap="control" key={row.label}>
              <Stack width={104}>
                <Text textRole="compact" tone="muted">
                  {row.label}
                </Text>
              </Stack>
              <Stack flexShrink={1} minWidth={0}>
                {row.value}
              </Stack>
            </Line>
          ))}
        </Stack>
      ) : null}
      {cast.length > 0 ? (
        <Stack aria-label="Cast" gap="control" role="region">
          <Text as="h2" textRole="headline">
            Cast
          </Text>
          <Line alignItems="flex-start" columnGap="control" rowGap="section">
            {cast.map((person) => (
              <Stack alignItems="center" key={`${person.name}:${person.character}`} width={104}>
                <Surface
                  alignItems="center"
                  aria-hidden
                  borderRadius={32}
                  height={64}
                  justifyContent="center"
                  overflow="hidden"
                  width={64}
                >
                  {person.photo ?? <Text textRole="title">{initials(person.name)}</Text>}
                </Surface>
                <PersonName center person={person} personLink={personLink} />
                {person.character ? (
                  <Text textAlign="center" textRole="compact" tone="muted">
                    {person.character}
                  </Text>
                ) : null}
              </Stack>
            ))}
          </Line>
        </Stack>
      ) : null}
    </>
  );
};

export { CastAndCrew, Hero, OnChannels, playingLine, useTitleLayout };

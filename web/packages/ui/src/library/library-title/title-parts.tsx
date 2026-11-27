import { Action, Surface, Text } from "@loomarr/design-system";
import { ChannelIdentity } from "../../identity";
import { Line, Stack } from "../library-poster";
import { lengthPhrase, picturePhrase, soundPhrase } from "../technical-details";
import type {
  LibraryTitleAiring,
  LibraryTitleDetail,
  LibraryTitlePerson,
  LibraryTitlePlaying,
  LibraryTitleProps,
} from "./library-title.type";

// The parts of the TV-app title page (#4476): the hero, the channels and the people.

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

// The backdrop with the title over its foot. The fade to the page keeps the title readable over any
// picture; with no backdrop the placeholder surface stands in.
const Hero = ({ title, wide }: { title: LibraryTitleDetail; wide: boolean }) => (
  <Stack>
    <Surface
      backgroundColor="artworkPlaceholder"
      borderWidth={0}
      height={wide ? 360 : 220}
      overflow="hidden"
      position="relative"
    >
      {title.backdrop}
      <Stack
        aria-hidden
        bottom={0}
        height="60%"
        left={0}
        pointerEvents="none"
        position="absolute"
        right={0}
        style={{ backgroundImage: "linear-gradient(transparent, var(--surfaceCanvas))" }}
      />
    </Surface>
    <Stack gap="inline" marginTop={-48} position="relative">
      {/* A logo stands for the name on screen; the heading still says it. */}
      <Text
        aria-label={title.logo ? title.name : undefined}
        as="h1"
        style={{ overflowWrap: "anywhere" }}
        textRole="display"
      >
        {title.logo ?? title.name}
      </Text>
      <Line gap="control">
        {heroFacts(title).map((fact) => (
          <Text key={fact} textRole="body" tone="secondary">
            {fact}
          </Text>
        ))}
        {title.rating ? (
          <Surface backgroundColor="transparent" paddingHorizontal="inline">
            <Text textRole="compact">{title.rating}</Text>
          </Surface>
        ) : null}
        {title.genres.length > 0 ? (
          <Text textRole="compact" tone="muted">
            {title.genres.join(", ")}
          </Text>
        ) : null}
        {title.score ? <Text textRole="body">{`Score ${title.score.toFixed(1)}/10`}</Text> : null}
      </Line>
      {title.tagline ? (
        <Text style={{ fontStyle: "italic" }} textRole="body">
          {title.tagline}
        </Text>
      ) : null}
    </Stack>
  </Stack>
);

// The page's main content: when each channel shows it next, earliest first, then the channels
// that hold it with nothing scheduled.
const OnChannels = ({ title }: { title: LibraryTitleDetail }) => {
  const airings = title.airings ?? [];
  const scheduled = new Set(airings.map((a) => a.channel.id));
  const rows: (LibraryTitleAiring | Pick<LibraryTitleAiring, "channel">)[] = [
    ...airings,
    ...title.channels.filter((c) => !scheduled.has(c.id)).map((channel) => ({ channel })),
  ];
  return (
    <Stack aria-label="On these channels" gap="control" role="region">
      <Text as="h2" textRole="headline">
        On these channels
      </Text>
      {rows.length === 0 ? (
        <Text textRole="body" tone="secondary">
          Not on a channel yet.
        </Text>
      ) : null}
      {rows.map((row) => (
        <Surface gap="inline" key={row.channel.id} padding="control">
          <ChannelIdentity
            channel={{
              channelLogoState: "missing",
              channelName: row.channel.name,
              channelNumber: String(row.channel.number),
            }}
          />
          {"when" in row ? (
            <Stack>
              <Text textRole="body">{`Next ${row.when}`}</Text>
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

const Person = ({
  person,
  onOpen,
}: {
  person: LibraryTitlePerson;
  onOpen: LibraryTitleProps["onOpenPerson"];
}) =>
  person.personId && onOpen ? (
    <Action density="touch" onPress={() => onOpen(person.personId as string)} variant="ghost">
      {person.name}
    </Action>
  ) : (
    <Text textRole="body">{person.name}</Text>
  );

const CastAndCrew = ({
  credits,
  onOpenPerson,
}: {
  credits: readonly LibraryTitlePerson[];
  onOpenPerson: LibraryTitleProps["onOpenPerson"];
}) => {
  const cast = credits.filter((c) => c.role === "actor");
  const crew = CREW.map(([role, label]) => ({
    label,
    people: credits.filter((c) => c.role === role),
  })).filter((group) => group.people.length > 0);
  if (credits.length === 0) return null;
  return (
    <Stack aria-label="Cast and crew" gap="control" role="region">
      <Text as="h2" textRole="headline">
        Cast and crew
      </Text>
      {crew.map((group) => (
        <Line key={group.label}>
          <Text textRole="compact" tone="muted">
            {group.label}
          </Text>
          {group.people.map((person) => (
            <Person key={`${person.personId}:${person.name}`} onOpen={onOpenPerson} person={person} />
          ))}
        </Line>
      ))}
      <Line alignItems="flex-start" columnGap="control" rowGap="section">
        {cast.map((person) => (
          <Stack alignItems="center" key={`${person.personId}:${person.character}`} width={112}>
            <Surface
              alignItems="center"
              aria-hidden
              borderRadius={32}
              height={64}
              justifyContent="center"
              width={64}
            >
              <Text textRole="title">{initials(person.name)}</Text>
            </Surface>
            <Person onOpen={onOpenPerson} person={person} />
            {person.character ? (
              <Text textAlign="center" textRole="compact" tone="muted">
                {person.character}
              </Text>
            ) : null}
          </Stack>
        ))}
      </Line>
    </Stack>
  );
};

export { CastAndCrew, Hero, OnChannels, playingLine };

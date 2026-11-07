import { formatBytes, languageLabel } from "@loomarr/core/format";
import { Disclosure, Text } from "@loomarr/design-system";
import { useState } from "react";

import { Line, Stack } from "../library-poster";
import type {
  LibraryTitleDetail,
  LibraryTitleFile,
  LibraryTitleMatch,
} from "../library-title/library-title.type";

// TECHNICAL DETAILS (#2151 step 10d, design/library "Settled in review" 3): the one place a title's
// technical facts live, still in whole words: where the details came from, the title match, then
// each file's path, size, picture, sound, subtitles and length.

// 5,880,000 ms → "1 h 38 min"; 2,580,000 → "43 min"; 20,000 → "Under a minute".
const lengthPhrase = (ms: number) => {
  const minutes = Math.round(ms / 60000);
  if (minutes < 1) return "Under a minute";
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return h > 0 ? `${h} h ${m} min` : `${m} min`;
};

// "Description and rating from TMDB. Episode names from Sonarr.": each source once, its details in
// the order the page shows them.
const originSentence = ({ description, rating, episodeNames }: LibraryTitleDetail["detailsFrom"]) => {
  const bySource = new Map<string, string[]>();
  for (const [what, source] of [
    ["description", description],
    ["rating", rating],
    ["episode names", episodeNames],
  ] as const) {
    if (source) bySource.set(source, [...(bySource.get(source) ?? []), what]);
  }
  return [...bySource]
    .map(([source, whats]) => {
      const list =
        whats.length > 1 ? `${whats.slice(0, -1).join(", ")} and ${whats.at(-1)}` : (whats[0] ?? "");
      return `${list.charAt(0).toUpperCase()}${list.slice(1)} from ${source}.`;
    })
    .join(" ");
};

const MATCH_WORDS: Record<LibraryTitleMatch["by"], (app?: string) => string> = {
  app: (app) => `Recognised from ${app ?? "the app that manages it"}`,
  file: () => "Recognised from the file",
  you: () => "You picked it",
  none: () => "Not recognised yet",
};

// "4K, extra-rich colour" · "1080p HD" · "Standard definition". Width counts too: a widescreen
// film cropped to 1920×800 is still 1080p.
const picturePhrase = ({ width, height, hdr }: NonNullable<LibraryTitleFile["picture"]>) => {
  const size =
    width >= 3200 || height >= 2000
      ? "4K"
      : width >= 1700 || height >= 1000
        ? "1080p HD"
        : width >= 1200 || height >= 700
          ? "720p HD"
          : "Standard definition";
  return hdr ? `${size}, extra-rich colour` : size;
};

const soundPhrase = (channels: number) =>
  channels === 1
    ? "Mono"
    : channels === 2
      ? "Stereo"
      : channels === 6
        ? "Surround 5.1"
        : channels === 8
          ? "Surround 7.1"
          : `${channels} channels`;

// "English, Spanish" · "None": each language once, a stream that doesn't say as "Unknown".
const subtitlesPhrase = (subtitles: LibraryTitleFile["subtitles"]) =>
  subtitles.length === 0
    ? "None"
    : [...new Set(subtitles.map((s) => languageLabel(s.language ?? "")))].join(", ");

const Row = ({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) => (
  <Line alignItems="flex-start" columnGap="section" rowGap={4}>
    <Stack width={208}>
      <Text textRole="compact" tone="muted">
        {label}
      </Text>
    </Stack>
    <Stack flexBasis={240} flexGrow={1} minWidth={0}>
      <Text textRole={mono ? "data" : "compact"}>{value}</Text>
    </Stack>
  </Line>
);

const FileRows = ({ file }: { file: LibraryTitleFile }) => (
  <Stack gap="control">
    <Row label="File" mono value={file.path} />
    <Row label="Size" value={formatBytes(file.size)} />
    {file.picture ? <Row label="Picture" value={picturePhrase(file.picture)} /> : null}
    {file.audioChannels ? <Row label="Sound" value={soundPhrase(file.audioChannels)} /> : null}
    {file.picture || file.audioChannels ? (
      <Row label="Subtitles" value={subtitlesPhrase(file.subtitles)} />
    ) : null}
    {file.lengthMillis ? <Row label="Length" value={lengthPhrase(file.lengthMillis)} /> : null}
  </Stack>
);

const TechnicalDetails = ({ title }: { title: LibraryTitleDetail }) => {
  const [open, setOpen] = useState(false);
  const origin = originSentence(title.detailsFrom);
  return (
    <Disclosure expanded={open} label="Technical details" onExpandedChange={setOpen}>
      <Stack gap="control">
        {origin ? <Row label="Where details come from" value={origin} /> : null}
        <Row label="Title match" value={MATCH_WORDS[title.match.by](title.match.app)} />
        {title.files.map((file) => (
          <FileRows file={file} key={file.path} />
        ))}
      </Stack>
    </Disclosure>
  );
};

export { lengthPhrase, TechnicalDetails };

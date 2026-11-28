import { formatBytes, languageLabel } from "@loomarr/core/format";
import { Action, Text } from "@loomarr/design-system";
import { useState } from "react";

import { Line, Stack } from "../library-poster";
import type { LibraryTitleFile, LibraryTitleStream } from "../library-title/library-title.type";

// ONE FILE'S FACTS (#4476): the plain summary first, then "More about this file" in place with what
// its header says. Technical details and each episode row's panel share it.

// 5,880,000 ms → "1 h 38 min"; 2,580,000 → "43 min"; 20,000 → "Under a minute".
const lengthPhrase = (ms: number) => {
  const minutes = Math.round(ms / 60000);
  if (minutes < 1) return "Under a minute";
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return h > 0 ? `${h} h ${m} min` : `${m} min`;
};

// A picture's quality class. Width counts too: a widescreen film cropped to 1920×800 is still 1080p.
const pictureClass = ({ width, height }: NonNullable<LibraryTitleFile["picture"]>) =>
  width >= 3200 || height >= 2000
    ? "4K"
    : width >= 1700 || height >= 1000
      ? "1080p"
      : width >= 1200 || height >= 700
        ? "720p"
        : "SD";

const PICTURE_WORDS = { "4K": "4K", "1080p": "1080p HD", "720p": "720p HD", SD: "Standard definition" };

// "4K, extra-rich colour" · "1080p HD" · "Standard definition".
const picturePhrase = (picture: NonNullable<LibraryTitleFile["picture"]>) => {
  const size = PICTURE_WORDS[pictureClass(picture)];
  return picture.hdr ? `${size}, extra-rich colour` : size;
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

const CONTAINERS: Record<string, string> = {
  "matroska,webm": "Matroska (MKV)",
  "mov,mp4,m4a,3gp,3g2,mj2": "MP4",
  mpegts: "MPEG transport stream",
  avi: "AVI",
};

const CODECS: Record<string, string> = {
  h264: "H.264",
  hevc: "H.265 (HEVC)",
  av1: "AV1",
  vp9: "VP9",
  mpeg2video: "MPEG-2",
  mpeg4: "MPEG-4",
  vc1: "VC-1",
  aac: "AAC",
  ac3: "AC-3",
  eac3: "E-AC-3",
  dts: "DTS",
  flac: "FLAC",
  opus: "Opus",
  mp3: "MP3",
  subrip: "SRT",
  ass: "ASS",
  hdmv_pgs_subtitle: "PGS (picture)",
  dvd_subtitle: "DVD (picture)",
  mov_text: "Timed text",
};

const codecName = (codec?: string) => (codec ? (CODECS[codec] ?? codec.toUpperCase()) : "");

// 9,800,000 bit/s → "9.8 Mb/s"; 640,000 → "640 kb/s".
const bitratePhrase = (bits: number) =>
  bits >= 1_000_000 ? `${(bits / 1_000_000).toFixed(1)} Mb/s` : `${Math.round(bits / 1000)} kb/s`;

// "24000/1001" → "23.976 frames a second".
const frameRatePhrase = (rate: string) => {
  const [top, bottom = "1"] = rate.split("/");
  const value = Number(top) / Number(bottom);
  return Number.isFinite(value) && value > 0
    ? `${Number(value.toFixed(3))} frames a second`
    : `${rate} frames a second`;
};

// 3,725,000 ms → "1:02:05".
const clockPhrase = (ms: number) => {
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
};

// A file by its name, with its folder only when another of `paths` has the same name.
const fileName = (path: string, paths: readonly string[] = []) => {
  const parts = path.split(/[\\/]/);
  const name = parts.pop() ?? path;
  const shared = paths.some((other) => other !== path && other.split(/[\\/]/).pop() === name);
  return shared && parts.length > 0 ? `${name} in ${parts.pop()}` : name;
};

const trackLanguage = (track: LibraryTitleStream) =>
  track.languageName || languageLabel(track.language ?? "");

const audioDetail = (track: LibraryTitleStream) =>
  [
    track.channels ? soundPhrase(track.channels) : (track.channelLayout ?? ""),
    track.audioFormat || codecName(track.codec),
    track.bitrate ? bitratePhrase(track.bitrate) : "",
    track.title ?? "",
    track.default ? "default" : "",
    track.commentary ? "commentary" : "",
    track.descriptive ? "describes the picture" : "",
    track.external ? "separate file" : "",
  ]
    .filter(Boolean)
    .join(" · ");

const subtitleDetail = (track: LibraryTitleStream) =>
  [
    track.forced ? "Forced" : "Full",
    codecName(track.codec),
    track.title ?? "",
    track.default ? "default" : "",
    track.hearingImpaired ? "for the deaf and hard of hearing" : "",
    track.commentary ? "commentary" : "",
    track.external ? "separate file" : "",
  ]
    .filter(Boolean)
    .join(" · ");

const videoPhrase = (track: LibraryTitleStream) =>
  [
    codecName(track.codec),
    track.profile ? `${track.profile} profile` : "",
    track.bitDepth ? `${track.bitDepth}-bit` : "",
  ]
    .filter(Boolean)
    .join(", ");

const Labelled = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <Line alignItems="flex-start" columnGap="section" rowGap={4}>
    <Stack width={208}>
      <Text textRole="compact" tone="muted">
        {label}
      </Text>
    </Stack>
    <Stack flexBasis={240} flexGrow={1} flexShrink={1} minWidth={0}>
      {children}
    </Stack>
  </Line>
);

const Row = ({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) => (
  <Labelled label={label}>
    <Text textRole={mono ? "data" : "compact"}>{value}</Text>
  </Labelled>
);

const Tracks = ({
  label,
  tracks,
  detail,
}: {
  label: string;
  tracks: readonly LibraryTitleStream[];
  detail: (track: LibraryTitleStream) => string;
}) => (
  <Labelled label={label}>
    {tracks.length === 0 ? <Text textRole="compact">None</Text> : null}
    {tracks.map((track) => (
      <Line columnGap="inline" key={track.index}>
        <Text textRole="compact">{trackLanguage(track)}</Text>
        <Text textRole="compact" tone="secondary">
          {detail(track)}
        </Text>
      </Line>
    ))}
  </Labelled>
);

// What the header says beyond the plain summary.
const MoreFacts = ({ file }: { file: LibraryTitleFile }) => {
  const streams = file.streams ?? [];
  const video = streams.find((s) => s.kind === "video");
  const bitrate = file.bitrate ?? video?.bitrate;
  const chapters = file.chapters ?? [];
  return (
    <Stack gap="inline">
      {file.container ? <Row label="Container" value={CONTAINERS[file.container] ?? file.container} /> : null}
      {video ? <Row label="Video" value={videoPhrase(video) || "Unknown"} /> : null}
      {bitrate ? <Row label="Bitrate" value={bitratePhrase(bitrate)} /> : null}
      {video?.frameRate ? <Row label="Frame rate" value={frameRatePhrase(video.frameRate)} /> : null}
      {video ? <Row label="HDR" value={video.dynamicRange ?? "None (standard range)"} /> : null}
      <Tracks detail={audioDetail} label="Sound tracks" tracks={streams.filter((s) => s.kind === "audio")} />
      <Tracks
        detail={subtitleDetail}
        label="Subtitle tracks"
        tracks={streams.filter((s) => s.kind === "subtitle")}
      />
      <Labelled label="Chapters">
        {chapters.length === 0 ? <Text textRole="compact">None</Text> : null}
        {chapters.map((chapter, i) => (
          <Text key={chapter.startMillis} textRole="compact">
            {`${clockPhrase(chapter.startMillis)} · ${chapter.name || `Chapter ${i + 1}`}`}
          </Text>
        ))}
      </Labelled>
    </Stack>
  );
};

// `label` names what the file is to the title ("File", "Extra copy", "Part 1 of 2").
const FileFacts = ({
  file,
  label = "File",
  density,
  defaultMore = false,
  name = fileName(file.path),
}: {
  file: LibraryTitleFile;
  label?: string;
  /** What the More button calls the file. */
  name?: string;
  density?: "pointer" | "touch";
  defaultMore?: boolean;
}) => {
  const [more, setMore] = useState(defaultMore);
  // An unread file has no header to tell more about.
  const read = (file.streams?.length ?? 0) > 0;
  return (
    <Stack gap="control">
      <Row label={label} mono value={file.path} />
      <Row label="Size" value={formatBytes(file.size)} />
      {file.picture ? <Row label="Picture" value={picturePhrase(file.picture)} /> : null}
      {file.audioChannels ? <Row label="Sound" value={soundPhrase(file.audioChannels)} /> : null}
      {file.picture || file.audioChannels ? (
        <Row label="Subtitles" value={subtitlesPhrase(file.subtitles)} />
      ) : null}
      {file.lengthMillis ? <Row label="Length" value={lengthPhrase(file.lengthMillis)} /> : null}
      {read ? (
        <Line>
          <Action
            // A title can list several files, so the name says which one; it starts with the
            // words on the button.
            accessibilityLabel={`${more ? "Less" : "More"} about this file, ${name}`}
            aria-expanded={more}
            density={density}
            icon={more ? "showLess" : "showMore"}
            onPress={() => setMore(!more)}
            variant="ghost"
          >
            {more ? "Less about this file" : "More about this file"}
          </Action>
        </Line>
      ) : null}
      {read && more ? <MoreFacts file={file} /> : null}
    </Stack>
  );
};

export { FileFacts, fileName, lengthPhrase, pictureClass, picturePhrase, Row, soundPhrase };

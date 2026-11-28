import { Disclosure } from "@loomarr/design-system";
import { useState } from "react";

import { FileFacts, fileName, lengthPhrase, picturePhrase, Row, soundPhrase } from "../file-facts";
import { fileState } from "../file-state";
import { Stack } from "../library-poster";
import type {
  LibraryTitleDetail,
  LibraryTitleFile,
  LibraryTitleMatch,
} from "../library-title/library-title.type";

// TECHNICAL DETAILS (#2151 step 10d, design/library "Settled in review" 3): the one place a title's
// technical facts live, still in whole words: where the details came from, the title match, then
// each file's facts.

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
  search: () => "Recognised by its name, year and length",
  none: () => "Not recognised yet",
};

// A file is named for what it is to the title, in the Copies card's words, after its place in a
// movie split over several files ("Part 1 of 2 · Plays").
const fileLabel = (file: LibraryTitleFile, parts: number): string => {
  const { words } = fileState(file);
  if (file.state !== "extra" && file.part && parts > 1) return `Part ${file.part} of ${parts} · ${words}`;
  return words;
};

const TechnicalDetails = ({
  title,
  density,
}: {
  title: LibraryTitleDetail;
  density: "pointer" | "touch";
}) => {
  const [open, setOpen] = useState(false);
  const origin = originSentence(title.detailsFrom);
  const parts = title.files.filter((file) => file.part && file.state !== "extra").length;
  return (
    <Disclosure density={density} expanded={open} label="Technical details" onExpandedChange={setOpen}>
      <Stack gap="control">
        {origin ? <Row label="Where details come from" value={origin} /> : null}
        <Row label="Title match" value={MATCH_WORDS[title.match.by](title.match.app)} />
        {title.files.map((file) => (
          <FileFacts
            density={density}
            file={file}
            key={file.path}
            label={fileLabel(file, parts)}
            name={fileName(
              file.path,
              title.files.map((f) => f.path),
            )}
          />
        ))}
      </Stack>
    </Disclosure>
  );
};

export { lengthPhrase, picturePhrase, soundPhrase, TechnicalDetails };

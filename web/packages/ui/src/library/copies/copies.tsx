import { formatBytes } from "@loomarr/core/format";
import { Action, Badge, Surface, Text } from "@loomarr/design-system";
import { type ComponentProps, useState } from "react";

import { pictureClass } from "../file-facts";
import { Line, Stack } from "../library-poster";
import type { LibraryTitleFile } from "../library-title/library-title.type";
import { useTitleLayout } from "../title-parts";

// A MOVIE'S COPIES (#4476, decisions 24 and 37): one card listing each file by quality, size and
// what it is to the title. Only an extra copy can be removed.

// A file whose header could not be read has no picture. An unread file has none yet either, and is
// not broken.
const isBroken = (file: LibraryTitleFile) =>
  file.state === "unplayable" || (file.state === "extra" && !file.picture && file.ready !== false);

const copyName = (file: LibraryTitleFile) => {
  const quality = file.picture ? pictureClass(file.picture) : undefined;
  const name = quality ? `${quality} copy` : isBroken(file) ? "A copy that can't be played" : "A copy";
  return file.part ? `${name}, part ${file.part}` : name;
};

const removeWords = (file: LibraryTitleFile) => {
  const copy = file.picture
    ? `Remove the ${pictureClass(file.picture)} copy`
    : isBroken(file)
      ? "Remove the broken copy"
      : "Remove the extra copy";
  return file.part ? `${copy} of part ${file.part}` : copy;
};

// Each extra's Remove names one file: its words, then its size where the words repeat, then its
// place among the copies that still match.
const removeNames = (extras: readonly LibraryTitleFile[]) => {
  const words = extras.map(removeWords);
  const sized = extras.map((file, i) =>
    words.filter((w) => w === words[i]).length > 1 ? `${words[i]}, ${formatBytes(file.size)}` : words[i],
  );
  return new Map(
    extras.map((file, i) => {
      const twins = sized.filter((s) => s === sized[i]).length;
      if (twins < 2) return [file.path, sized[i] === words[i] ? undefined : sized[i]];
      const place = sized.slice(0, i + 1).filter((s) => s === sized[i]).length;
      return [file.path, `${sized[i]}, copy ${place} of ${twins}`];
    }),
  );
};

type Tone = ComponentProps<typeof Badge>["tone"];

const chipOf = (file: LibraryTitleFile): [Tone, string] => {
  if (isBroken(file)) return ["danger", "Can't be played"];
  switch (file.state) {
    case "plays":
      return ["success", "Plays"];
    case "extra":
      return ["warning", "Extra"];
    case "reading":
      return ["neutral", "Reading"];
    default:
      return ["neutral", "Waiting"];
  }
};

// The row stays until the page refetches without the file; a press while one is out is dropped.
const Copy = ({
  file,
  accessibilityLabel,
  onRemove,
}: {
  file: LibraryTitleFile;
  /** Set when another row's button reads the same. */
  accessibilityLabel?: string;
  onRemove: (path: string) => Promise<void>;
}) => {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const { density } = useTitleLayout();
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
  const [tone, state] = chipOf(file);
  return (
    <Stack gap="inline" role="listitem">
      <Line columnGap="control" rowGap="inline">
        <Line columnGap="control" flexBasis={240} flexGrow={1} minWidth={0}>
          <Text textRole="body">{copyName(file)}</Text>
          <Text textRole="compact" tone="muted">
            {formatBytes(file.size)}
          </Text>
          <Badge tone={tone}>{state}</Badge>
        </Line>
        {file.state === "extra" ? (
          <Action
            accessibilityLabel={accessibilityLabel}
            density={density}
            onPress={remove}
            variant="outline"
          >
            {removeWords(file)}
          </Action>
        ) : null}
      </Line>
      {failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't remove it. Loomarr may not be allowed to change this folder.
        </Text>
      ) : null}
    </Stack>
  );
};

/** Whether a movie's files need the card: more than one, or one that can't be played. */
const needsCopies = (files: readonly LibraryTitleFile[]) => files.length > 1 || files.some(isBroken);

const Copies = ({
  files,
  onRemove,
}: {
  files: readonly LibraryTitleFile[];
  onRemove: (path: string) => Promise<void>;
}) => {
  const extras = files.filter((file) => file.state === "extra");
  const attention = extras.length > 0 || files.some(isBroken);
  const names = removeNames(extras);
  return (
    <Surface borderColor={attention ? "stateWarning" : undefined} gap="control" padding="section">
      <Text as="h2" textRole="title">
        Copies
      </Text>
      {extras.length > 0 ? (
        <Text textRole="body" tone="secondary">
          Loomarr plays another copy in place of an extra one, so an extra copy never plays. Removing it moves
          it to Loomarr's trash. You can undo right after. After that, the file is in the .loomarr-trash
          folder inside its media folder for 30 days.
        </Text>
      ) : null}
      <Stack gap="control" role="list">
        {files.map((file) => (
          <Copy accessibilityLabel={names.get(file.path)} file={file} key={file.path} onRemove={onRemove} />
        ))}
      </Stack>
    </Surface>
  );
};

export { Copies, needsCopies };

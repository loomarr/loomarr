import { formatBytes } from "@loomarr/core/format";
import { Action, Badge, Surface, Text } from "@loomarr/design-system";
import { type ComponentRef, type RefObject, useEffect, useRef, useState } from "react";
import { View } from "react-native";

import { fileName, pictureClass } from "../file-facts";
import { fileState, isBrokenFile } from "../file-state";
import { Line, Stack } from "../library-poster";
import type { LibraryTitleFile } from "../library-title/library-title.type";
import { useTitleLayout } from "../title-parts";

// A MOVIE'S COPIES (#4476, decisions 24 and 37): one card listing each file by quality, size and
// what it is to the title. Only an extra copy can be removed.

// What takes focus: a button, or a heading's wrapper.
type Focusable = { focus: () => void };

const copyName = (file: LibraryTitleFile) => {
  const quality = file.picture ? pictureClass(file.picture) : undefined;
  const name = quality ? `${quality} copy` : isBrokenFile(file) ? "A copy that can't be played" : "A copy";
  return file.part ? `${name}, part ${file.part}` : name;
};

const removeWords = (file: LibraryTitleFile) => {
  const copy = file.picture
    ? `Remove the ${pictureClass(file.picture)} copy`
    : isBrokenFile(file)
      ? "Remove the broken copy"
      : "Remove the extra copy";
  return file.part ? `${copy} of part ${file.part}` : copy;
};

const TAIL = 10;

const baseName = (path: string) => path.split(/[\\/]/).pop() ?? path;

// The shortest end of each file's name, from TAIL characters, that sets it apart from the others.
// Files with one name in different folders are named with their folder.
const nameTails = (paths: readonly string[]) => {
  const names = paths.map(baseName);
  const end = (name: string, length: number) =>
    name.length > length ? `…${name.slice(-length).trim()}` : name;
  return paths.map((path, i) => {
    const name = names[i] ?? path;
    if (names.filter((other) => other === name).length > 1) return fileName(path, paths);
    let length = TAIL;
    while (
      length < name.length &&
      names.some((other, j) => j !== i && end(other, length) === end(name, length))
    )
      length += 1;
    return end(name, length);
  });
};

// Rows that read alike, by quality and size, are told apart by the end of their file names.
const collisionTails = (files: readonly LibraryTitleFile[]) => {
  const alike = new Map<string, string[]>();
  for (const file of files) {
    if (!file.picture) continue;
    const reads = `${copyName(file)}|${formatBytes(file.size)}`;
    alike.set(reads, [...(alike.get(reads) ?? []), file.path]);
  }
  const tails = new Map<string, string>();
  for (const paths of alike.values()) {
    if (paths.length < 2) continue;
    nameTails(paths).forEach((tail, i) => {
      tails.set(paths[i] ?? "", tail);
    });
  }
  return tails;
};

// Each extra's Remove names one file: its words and its file name's end where two rows read alike.
// Otherwise its words, then its size where the words repeat, then its place among the copies that
// still match.
const removeNames = (extras: readonly LibraryTitleFile[], tails: ReadonlyMap<string, string>) => {
  const words = extras.map((file) => {
    const tail = tails.get(file.path);
    return tail ? `${removeWords(file)} ${tail}` : removeWords(file);
  });
  const sized = extras.map((file, i) =>
    words.filter((w) => w === words[i]).length > 1 ? `${words[i]}, ${formatBytes(file.size)}` : words[i],
  );
  return new Map(
    extras.map((file, i) => {
      const twins = sized.filter((s) => s === sized[i]).length;
      if (twins < 2) {
        return [file.path, sized[i] === removeWords(file) ? undefined : sized[i]];
      }
      const place = sized.slice(0, i + 1).filter((s) => s === sized[i]).length;
      return [file.path, `${sized[i]}, copy ${place} of ${twins}`];
    }),
  );
};

// FOCUS AFTER A REMOVAL: the pressed button leaves with its row once the page refetches without
// the file. Focus then goes to the next copy's Remove, else the Copies heading, else (the card is
// gone too) the page heading. A removal that fails leaves focus where it is.
type ViewRef = RefObject<ComponentRef<typeof View> | null>;

interface CopyFocus {
  cardHeading: ViewRef;
  pageHeading: ViewRef;
  holdRemove: (path: string) => (node: unknown) => void;
  removing: (path: string) => void;
  failed: (path: string) => void;
}

const useCopyFocus = (files: readonly LibraryTitleFile[]): CopyFocus => {
  const removes = useRef(new Map<string, Focusable>());
  const cardHeading = useRef<ComponentRef<typeof View>>(null);
  const pageHeading = useRef<ComponentRef<typeof View>>(null);
  const pending = useRef<{ path: string; next: string[] } | undefined>(undefined);

  useEffect(() => {
    const out = pending.current;
    if (!out || files.some((file) => file.path === out.path)) return;
    pending.current = undefined;
    const next = out.next.map((path) => removes.current.get(path)).find(Boolean);
    const target =
      next ?? (cardHeading.current as Focusable | null) ?? (pageHeading.current as Focusable | null);
    target?.focus();
  }, [files]);

  return {
    cardHeading,
    pageHeading,
    holdRemove: (path: string) => (node: unknown) => {
      if (node) removes.current.set(path, node as Focusable);
      else removes.current.delete(path);
    },
    removing: (path: string) => {
      const after = files.slice(files.findIndex((file) => file.path === path) + 1);
      pending.current = {
        path,
        next: after.filter((file) => file.state === "extra").map((file) => file.path),
      };
    },
    failed: (path: string) => {
      if (pending.current?.path === path) pending.current = undefined;
    },
  };
};

// The row stays until the page refetches without the file; a press while one is out is dropped.
const Copy = ({
  file,
  accessibilityLabel,
  tail,
  focus,
  onRemove,
}: {
  file: LibraryTitleFile;
  /** Set when another row's button reads the same. */
  accessibilityLabel?: string;
  /** The end of the file's name, where another row has its quality and size. */
  tail?: string;
  focus: CopyFocus;
  onRemove: (path: string) => Promise<void>;
}) => {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const { density } = useTitleLayout();
  const remove = () => {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    focus.removing(file.path);
    onRemove(file.path).then(
      () => setBusy(false),
      () => {
        focus.failed(file.path);
        setBusy(false);
        setFailed(true);
      },
    );
  };
  const { tone, words } = fileState(file);
  return (
    <Stack gap="inline" role="listitem">
      <Line columnGap="control" rowGap="inline">
        <Line columnGap="control" flexBasis={240} flexGrow={1} minWidth={0}>
          <Text textRole="body">{copyName(file)}</Text>
          <Text textRole="compact" tone="muted">
            {formatBytes(file.size)}
          </Text>
          {tail ? <Text textRole="data">{tail}</Text> : null}
          <Badge tone={tone}>{words}</Badge>
        </Line>
        {file.state === "extra" ? (
          <Action
            accessibilityLabel={accessibilityLabel}
            density={density}
            onPress={remove}
            ref={focus.holdRemove(file.path)}
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
const needsCopies = (files: readonly LibraryTitleFile[]) => files.length > 1 || files.some(isBrokenFile);

const Copies = ({
  files,
  focus,
  onRemove,
}: {
  files: readonly LibraryTitleFile[];
  focus: CopyFocus;
  onRemove: (path: string) => Promise<void>;
}) => {
  const extras = files.filter((file) => file.state === "extra");
  const attention = extras.length > 0 || files.some(isBrokenFile);
  const tails = collisionTails(files);
  const names = removeNames(extras, tails);
  return (
    <Surface borderColor={attention ? "stateWarning" : undefined} gap="control" padding="section">
      {/* Takes focus from script only, after the last removable copy goes. */}
      <View ref={focus.cardHeading} tabIndex={-1}>
        <Text as="h2" textRole="title">
          Copies
        </Text>
      </View>
      {extras.length > 0 ? (
        <Text textRole="body" tone="secondary">
          Loomarr plays another copy in place of an extra one, so an extra copy never plays. Removing it moves
          it to Loomarr's trash. You can undo right after. After that, the file is in the .loomarr-trash
          folder inside its media folder for 30 days.
        </Text>
      ) : null}
      <Stack gap="control" role="list">
        {files.map((file) => (
          <Copy
            accessibilityLabel={names.get(file.path)}
            file={file}
            focus={focus}
            key={file.path}
            onRemove={onRemove}
            tail={tails.get(file.path)}
          />
        ))}
      </Stack>
    </Surface>
  );
};

export { Copies, needsCopies, useCopyFocus };

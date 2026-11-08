// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Small pieces both new grammars share: the status (a dot and a word), the channel chip, the one
// metadata line, and the specimen panel that frames a surface.
import { Action, StatusDot, Text } from "@loomarr/design-system";
import { Archive, FolderOpen, Newspaper } from "lucide-react";
import { Fragment, type ReactNode } from "react";
import { eraTint, type Tint } from "../shared";
import type { Chan, SourceItem, Status } from "./data";

const tintOf = (chan: Chan | undefined, tint: Tint) => (chan ? eraTint(chan.era, tint) : undefined);

const StatusLine = ({ status }: { status: Status }) => (
  <span className="inline-flex items-center gap-1.5">
    <StatusDot label="" tone={status.tone} />
    <Text textRole="cardMeta" tone="secondary">
      {status.label}
    </Text>
  </span>
);

// "03 Late Night Sci-Fi". Neutral = grey chrome; era = the channel's colour on border and wash.
const ChannelChip = ({ chan, tint }: { chan: Chan; tint: Tint }) => {
  const color = tintOf(chan, tint);
  return (
    <span
      className="bg-static-800 inline-flex max-w-full items-center gap-1.5 rounded px-1.5 py-0.5"
      style={
        color
          ? {
              backgroundColor: `color-mix(in srgb, ${color} 16%, transparent)`,
              boxShadow: `inset 0 0 0 1px color-mix(in srgb, ${color} 55%, transparent)`,
            }
          : undefined
      }
    >
      <Text textRole="cardChannelNumber">{chan.number}</Text>
      <span className="min-w-0 truncate whitespace-nowrap">
        <Text numberOfLines={1} textRole="cardMeta" tone="secondary">
          {chan.name}
        </Text>
      </span>
    </span>
  );
};

// A plain kind chip for items that have no channel (source kind, person role) in the one-card
// grammar: same chip slot, no colour.
const KindChip = ({ children }: { children: ReactNode }) => (
  <span className="bg-static-800 inline-flex items-center rounded px-1.5 py-0.5">
    <Text textRole="cardMeta" tone="secondary">
      {children}
    </Text>
  </span>
);

// The one metadata line: parts joined by a middle dot, wrapping on a narrow screen.
const MetaLine = ({ parts }: { parts: ReactNode[] }) => (
  <div className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5">
    {parts.map((part, i) => (
      // biome-ignore lint/suspicious/noArrayIndexKey: static specimen
      <Fragment key={i}>
        {i > 0 && (
          <Text textRole="cardMeta" tone="secondary" aria-hidden>
            ·
          </Text>
        )}
        {typeof part === "string" ? (
          <Text textRole="cardMeta" tone="secondary">
            {part}
          </Text>
        ) : (
          part
        )}
      </Fragment>
    ))}
  </div>
);

// The one action at the right edge. Quiet at rest; secondary when the item needs attention.
const CardAction = ({ label, attention }: { label: string; attention?: boolean }) => (
  <div className="shrink-0">
    <Action accessibilityRole="button" variant={attention ? "secondary" : "ghost"}>
      {label}
    </Action>
  </div>
);

const SourceIcon = ({ icon }: { icon: SourceItem["icon"] }) => {
  const cls = "text-muted-foreground size-5";
  return icon === "folder" ? (
    <FolderOpen aria-hidden className={cls} />
  ) : icon === "archive" ? (
    <Archive aria-hidden className={cls} />
  ) : (
    <Newspaper aria-hidden className={cls} />
  );
};

const initialOf = (name: string) => name.slice(0, 1).toUpperCase();

// A specimen panel: a plain label and the surface under it. The label is the maintainer's, not the
// app's, so it is small and quiet.
const Panel = ({ title, note, children }: { title: string; note?: string; children: ReactNode }) => (
  <section className="flex flex-col gap-3">
    <div className="flex items-baseline gap-3">
      <Text as="h2" textRole="label">
        {title}
      </Text>
      {note ? (
        <Text textRole="cardMeta" tone="secondary">
          {note}
        </Text>
      ) : null}
    </div>
    {children}
  </section>
);

export { CardAction, ChannelChip, initialOf, KindChip, MetaLine, Panel, SourceIcon, StatusLine, tintOf };

import * as channelsApi from "@loomarr/api/endpoints/channels";
import type { ChannelPolicy } from "@loomarr/api/models/channelPolicy";
import type { ProgrammingChangeDTO } from "@loomarr/api/models/programmingChangeDTO";
import type { ProgrammingChangeSideDTO } from "@loomarr/api/models/programmingChangeSideDTO";
import { ProgrammingChangeSideDTOKind } from "@loomarr/api/models/programmingChangeSideDTOKind";
import { unwrap } from "@loomarr/api/unwrap";
import { formatGuideEpisode, formatGuideTime } from "@loomarr/core/guide";
import { useEffect } from "react";
import { PREVIEW_DEBOUNCE_MS } from "@/channels/use-channel-rules-draft";
import { cn } from "@/lib/utils";
import { EmptyState, ErrorState } from "../../feedback";
import type { ChannelProgrammingChangesProps } from "./channel-programming-changes.type";

// One weekday formatter per zone, beside the guide's own clock formatter: the span can run to a
// week, so a bare "6:00 PM" would not say which day.
const weekdayFormatters = new Map<string | undefined, Intl.DateTimeFormat>();
const formatSlotTime = (ms: number, timeZone?: string): string => {
  let weekday = weekdayFormatters.get(timeZone);
  if (!weekday) {
    weekday = new Intl.DateTimeFormat("en-US", { weekday: "short", timeZone });
    weekdayFormatters.set(timeZone, weekday);
  }
  return `${weekday.format(ms)} ${formatGuideTime(ms, timeZone)}`;
};

// What one side airs, in the guide's words. An absent side means nothing airable airs there.
const sideLabel = (side: ProgrammingChangeSideDTO | undefined): string => {
  if (!side) return "Nothing airs";
  if (side.kind === ProgrammingChangeSideDTOKind.break) return "Commercial break";
  const title = side.title || "Untitled";
  if (!side.series) return title;
  const episode = formatGuideEpisode(side.season, side.episode);
  return `${side.series}${episode ? ` · ${episode}` : ""} — ${title}`;
};

const ChangeRow = ({ change, timeZone }: { change: ProgrammingChangeDTO; timeZone?: string }) => (
  <li className="flex flex-wrap items-baseline gap-x-2.5 gap-y-0.5 border-border border-b py-1.5 text-sm last:border-b-0">
    <span className="w-28 shrink-0 font-mono text-muted-foreground">
      {formatSlotTime(change.startMs, timeZone)}
    </span>
    <span className="min-w-0 flex-1">
      <span className="text-muted-foreground line-through">{sideLabel(change.before)}</span>
      <span className="text-muted-foreground" aria-hidden>
        {" → "}
      </span>
      <span className="sr-only"> becomes </span>
      <span>{sideLabel(change.after)}</span>
    </span>
    {change.after ? (
      <span className="ml-auto text-muted-foreground text-xs">{change.after.rule.label}</span>
    ) : null}
  </li>
);

// ChannelProgrammingChanges — the upcoming slots an unsaved edit changes (programming-design §8.1,
// #1882). It compares the draft against the saved channel over the channel's schedule window, so
// it answers "what will this edit change on air, and when" where the cycle preview answers "what
// airs at one moment".
const ChannelProgrammingChanges = ({
  channelId,
  draftPolicy,
  timeZone,
  className,
}: ChannelProgrammingChangesProps) => {
  const changes = channelsApi.usePreviewChannelProgrammingChanges();

  // The same debounce and serialized key as the cycle preview, so both settle together. `from`
  // is left out so the server resolves "now".
  const draftKey = draftPolicy ? JSON.stringify(draftPolicy) : "";
  // biome-ignore lint/correctness/useExhaustiveDependencies: draftKey IS the dependency
  useEffect(() => {
    if (!draftKey) return;
    const t = setTimeout(() => {
      changes.mutate({ id: channelId, data: { policy: JSON.parse(draftKey) as ChannelPolicy } });
    }, PREVIEW_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [channelId, draftKey]);

  const body = unwrap(changes.data);

  let content: React.ReactNode;
  if (!draftPolicy) {
    content = (
      <p className="text-muted-foreground text-sm">
        Edit the rules above to see which upcoming slots change.
      </p>
    );
  } else if (changes.error) {
    content = (
      <ErrorState
        error={changes.error}
        onRetry={() => changes.mutate({ id: channelId, data: { policy: draftPolicy } })}
      />
    );
  } else if (changes.isPending || !body) {
    // Also covers the debounce window before the first request, which has no data yet.
    content = <p className="text-muted-foreground text-sm">Loading changes…</p>;
  } else if (body.compared === 0) {
    content = (
      <EmptyState title="Nothing scheduled" description="Nothing airs in this span to compare against." />
    );
  } else if (body.count === 0) {
    content = <EmptyState title="No upcoming slots change" />;
  } else {
    content = (
      <div className="flex flex-col gap-2">
        <p className="text-muted-foreground text-xs">
          {body.truncated
            ? `Showing ${body.changes.length} of ${body.count} changes`
            : `${body.count} ${body.count === 1 ? "change" : "changes"}`}{" "}
          until {formatSlotTime(body.toMs, timeZone)}
        </p>
        {/* Rows are text, so the scroll region itself takes the Tab stop (see the cycle
            preview's slot list for the same treatment). */}
        <ul
          className="scroll-thin flex max-h-96 flex-col overflow-y-auto"
          // biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable region IS interactive
          tabIndex={0}
          aria-label="Changed slots"
        >
          {body.changes.map((change) => (
            <ChangeRow key={change.startMs} change={change} timeZone={timeZone} />
          ))}
        </ul>
      </div>
    );
  }

  return (
    <section aria-labelledby="programming-changes-heading" className={cn("flex flex-col gap-2", className)}>
      <h3 id="programming-changes-heading" className="font-semibold text-base">
        What changes
      </h3>
      {content}
    </section>
  );
};

export { ChannelProgrammingChanges, formatSlotTime, sideLabel };

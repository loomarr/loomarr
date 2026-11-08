// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Variant A — Today. The real ClipCard, the real ChannelIdent, and faithful copies of the On now
// card, the request card (packages/ui RequestCard needs a journey record, so it is copied, not
// imported), the filler source row and the People row / invitation card. Layout switches on the
// frame's device, not the browser width, because the frame is the "screen".
import type { ClipDTO } from "@loomarr/api/models/clipDTO";
import { Action, Badge, type BadgeTone, Surface, Text } from "@loomarr/design-system";
import { ChevronRight, KeyRound, Mail, QrCode, Server } from "lucide-react";
import { ChannelIdent } from "@/components/loomarr/channels/channel-ident";
import { ClipCard } from "@/components/loomarr/filler";
import { Button } from "@/components/ui/button";
import { standInImage } from "../shared";
import { clips, PROGRAMMES, requests, type State, sources } from "./data";
import { Panel } from "./parts";
import type { SurfaceProps } from "./specimen";

const OnNowToday = ({ phone }: { phone: boolean }) => (
  <div className={`grid gap-3 ${phone ? "grid-cols-1" : "grid-cols-3"}`}>
    {PROGRAMMES.map((p) => (
      <div
        key={p.id}
        className="border-static-700 bg-static-900 block min-w-0 overflow-hidden rounded-lg border"
      >
        <div className="bg-static-800 relative aspect-video w-full overflow-hidden">
          <img alt="" className="size-full object-cover" src={standInImage(p.hue).src} />
        </div>
        <div className="flex flex-col gap-1.5 p-3.5">
          <div className="flex items-center gap-2">
            <ChannelIdent name={p.channel.name} number={Number(p.channel.number)} size={28} />
            <span className="text-foreground font-mono text-sm">{p.channel.number}</span>
            <span className="text-static-400 truncate text-xs">{p.channel.name}</span>
          </div>
          <h3 className="m-0 text-base font-semibold break-words">{p.title}</h3>
          {p.episode && <p className="text-static-400 m-0 text-xs">{p.episode}</p>}
          <p className="text-static-400 m-0 font-mono text-xs">{p.time}</p>
          <span className="text-foreground text-sm font-medium">
            Watch live <span aria-hidden="true">→</span>
          </span>
        </div>
      </div>
    ))}
  </div>
);

const REQUEST_TONE: Record<string, BadgeTone> = {
  attention: "warning",
  ok: "success",
  progress: "info",
};

// RequestCard's body: a status badge, the brief in two lines, the detail, when it was asked, and the
// one action as a full-width secondary button. No artwork slot.
const RequestToday = ({ state }: { state: State }) => (
  <div className="flex max-w-xl flex-col gap-3">
    {requests(state).map((r) => (
      <Surface gap="control" key={r.id} level="raised" padding="control" width="100%">
        <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
          <Badge density="touch" tone={REQUEST_TONE[r.status.tone] ?? "neutral"}>
            {r.status.label}
          </Badge>
          <Text density="touch" numberOfLines={2} textRole="title">
            {r.title}
          </Text>
          {r.channel ? (
            <Text density="touch" textRole="body" tone="secondary">
              For {r.channel.name}
            </Text>
          ) : null}
          <Text density="touch" textRole="caption" tone="secondary">
            {r.meta}
          </Text>
        </Surface>
        <Action accessibilityRole="button" density="touch" tone="secondary">
          {r.action}
        </Action>
      </Surface>
    ))}
  </div>
);

const clipDto = (c: ReturnType<typeof clips>[number], state: State): ClipDTO => ({
  audience: "family",
  category: c.kind === "Commercial" ? "food & drink" : "",
  durationMs: c.duration === "0:10" ? 10000 : 30000,
  era: Number.parseInt(c.era, 10),
  hash: `hash-${c.id}`,
  name: c.title,
  // Today the card has no "failed quality check" state; the closest exception it can draw is the
  // server's needsDetails chip.
  needsDetails: state === "problems" && c.status !== undefined,
  playCount: c.meta.includes("never") ? 0 : 6,
  playsCounted: true,
  role: c.kind === "Station ID" ? "station_id" : "commercial",
  tagged: true,
  thumbImage: standInImage(c.hue, undefined, 640, 360),
});

const ClipsToday = ({ phone, state }: { phone: boolean; state: State }) => (
  <div className={`grid gap-3 ${phone ? "grid-cols-2" : "grid-cols-3"}`}>
    {clips(state)
      .slice(0, 3)
      .map((c) => (
        <ClipCard key={c.id} clip={clipDto(c, state)} onOpen={() => {}} compact={phone} />
      ))}
  </div>
);

// FillerSources' row: name with a caution badge when it needs attention, a detail line, then the
// status text and a chevron. The whole row is the control.
const SourcesToday = ({ phone, state }: { phone: boolean; state: State }) => (
  <ul className="flex flex-col gap-1">
    {sources(state).map((s) => {
      const stale = state === "problems" && s.id === "s3";
      return (
        <li key={s.id} className="border-border/50 border-b py-1 last:border-b-0">
          <div
            className={`flex min-w-0 flex-1 items-center gap-3 px-2 py-2 ${phone ? "flex-wrap" : "flex-nowrap"}`}
          >
            <div className={`min-w-0 text-left ${phone ? "basis-full" : "flex-1"}`}>
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <span className="truncate text-sm font-medium">{s.title}</span>
                {stale && (
                  <Badge shape="square" tone="caution">
                    Needs attention
                  </Badge>
                )}
              </div>
              <p className="text-muted-foreground mt-0.5 truncate text-xs">
                {stale ? "Hasn’t checked in lately." : `A ${s.kind.toLowerCase()} Loomarr watches for clips.`}
              </p>
            </div>
            <span className="text-muted-foreground shrink-0 text-xs whitespace-nowrap">{s.meta}</span>
            <ChevronRight className="text-muted-foreground size-4 shrink-0" aria-hidden />
          </div>
        </li>
      );
    })}
  </ul>
);

const ROSTER = [
  { local: false, name: "Ada Marlowe", role: "admin", seen: "Today", quota: "0 / 5", you: true },
  { local: true, name: "Theo Brightwater", role: "member", seen: "Yesterday", quota: "1 / 3", you: false },
  { local: true, name: "Juniper Oake", role: "member", seen: "3 days ago", quota: "0 / 3", you: false },
];

const COLS = "grid-cols-[minmax(0,2fr)_8rem_8rem_8rem_8rem]";

const PeopleToday = ({ phone, state }: { phone: boolean; state: State }) => {
  const rows = state === "problems" ? ROSTER.slice(0, 2) : ROSTER;
  return (
    <div className="flex flex-col gap-3">
      <div className="border-border bg-card overflow-hidden rounded-lg border">
        {!phone && (
          <div
            aria-hidden
            className={`border-static-800 text-static-400 grid ${COLS} gap-4 border-b px-4 py-2 font-mono text-xs tracking-wide uppercase`}
          >
            <span>Person</span>
            <span>Role</span>
            <span>Requests</span>
            <span>Status</span>
            <span>Last seen</span>
          </div>
        )}
        <ul>
          {rows.map((u) => (
            <li key={u.name} className="border-static-800 border-b last:border-b-0">
              <div
                className={`relative grid gap-3 p-4 text-left ${phone ? "grid-cols-1" : `${COLS} items-center`}`}
              >
                <span className="min-w-0">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="truncate font-medium">{u.name}</span>
                    {u.you && (
                      <Badge shape="square" tone="tune">
                        You
                      </Badge>
                    )}
                  </span>
                  <span className="text-static-400 mt-1 flex items-center gap-1.5 text-xs">
                    {u.local ? (
                      <KeyRound className="size-3" aria-hidden />
                    ) : (
                      <Server className="size-3" aria-hidden />
                    )}
                    {u.local ? "Local account" : "Media-server account · offline login ready"}
                  </span>
                </span>
                <span className="flex items-center justify-between gap-3">
                  {phone && <span className="text-muted-foreground text-xs">Role</span>}
                  <Badge shape="square" tone={u.role === "admin" ? "tune" : "neutral"}>
                    {u.role}
                  </Badge>
                </span>
                <span className="flex items-center justify-between gap-3 font-mono text-sm tabular-nums">
                  {phone && <span className="text-muted-foreground font-sans text-xs">Requests</span>}
                  {u.quota}
                </span>
                <span className="flex items-center justify-between gap-3">
                  {phone && <span className="text-muted-foreground text-xs">Status</span>}
                  <Badge shape="square">Enabled</Badge>
                </span>
                <span className="flex items-center justify-between gap-3">
                  {phone && <span className="text-muted-foreground text-xs">Last seen</span>}
                  <span className="text-muted-foreground font-mono text-xs">{u.seen}</span>
                </span>
              </div>
            </li>
          ))}
        </ul>
      </div>
      {state === "problems" && (
        // An expired invitation lives in the separate Invitations list, with its own card.
        <div className="border-border bg-card rounded-lg border p-4">
          <div className={`flex gap-4 ${phone ? "flex-col" : "items-start justify-between"}`}>
            <div className="min-w-0 space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">Juniper Oake</span>
                <Badge shape="square">member</Badge>
                <Badge shape="square" tone="onair">
                  expired
                </Badge>
              </div>
              <p className="text-static-400 text-sm">juniper@example.test</p>
              <p className="text-muted-foreground max-w-3xl text-sm">
                Loomarr will send this account message shortly.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button type="button" size="sm" variant="outline">
                <Mail aria-hidden />
                Resend email
              </Button>
              <Button type="button" size="sm" variant="ghost">
                <QrCode className="size-4" aria-hidden /> Share QR or link
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

const Today = ({ state, phone }: SurfaceProps & { state: State }) => (
  <>
    <Panel title="On now" note="the real card layout">
      <OnNowToday phone={phone} />
    </Panel>
    <Panel title="Request item" note="no picture; badge on top; full-width action">
      <RequestToday state={state} />
    </Panel>
    <Panel title="Clip card" note="the real ClipCard">
      <ClipsToday phone={phone} state={state} />
    </Panel>
    <Panel title="Filler source row" note="name, detail, status text, chevron">
      <SourcesToday phone={phone} state={state} />
    </Panel>
    <Panel title="People row" note="roster table; invitations are a separate card">
      <PeopleToday phone={phone} state={state} />
    </Panel>
  </>
);

export { Today };

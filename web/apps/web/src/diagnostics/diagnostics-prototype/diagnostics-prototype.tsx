// PROTOTYPE — THROW AWAY AFTER #537.
// Three Diagnostics layouts, switchable with ?variant=, on the existing route.
import {
  Activity,
  ChevronDown,
  CirclePause,
  Download,
  FileArchive,
  HeartPulse,
  ListFilter,
  Radio,
  Search,
  Server,
  Smartphone,
  Tv,
} from "lucide-react";
import { useState } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusDot } from "@/components/ui/status-dot";
import { PrototypeSwitcher, type PrototypeVariant } from "./prototype-switcher";

type PrototypeEvent = {
  id: string;
  time: string;
  level: "error" | "warn" | "info";
  source: "Loomarr" | "Web player" | "Android TV" | "FFmpeg";
  message: string;
  context: string;
  technical: string;
  process?: boolean;
};

const events: PrototypeEvent[] = [
  {
    id: "handoff",
    time: "10:42:19 PM",
    level: "error",
    source: "Web player",
    message: "The commercial break did not become playable before the handoff.",
    context: "Channel 7 · Commercial break · 2.1 seconds late",
    technical: "player.transition_failed · playback play_01K3D8J2 · process process-ffmpeg-19",
    process: true,
  },
  {
    id: "late",
    time: "10:42:17 PM",
    level: "warn",
    source: "Loomarr",
    message: "The schedule moved forward while the previous segment was still finishing.",
    context: "Channel 7 · Commercial break · 1.8 seconds late",
    technical: "playout.transition_late · block-commercial-42",
  },
  {
    id: "buffer",
    time: "10:42:16 PM",
    level: "warn",
    source: "Android TV",
    message: "Playback buffered while waiting for the next segment.",
    context: "Channel 7 · Living room TV · 1.4 seconds",
    technical: "player.buffering · session play_01K3D8J2 · Media3",
  },
  {
    id: "ffmpeg",
    time: "10:42:13 PM",
    level: "info",
    source: "FFmpeg",
    message: "Started preparing the commercial break stream.",
    context: "Channel 7 · Process still available",
    technical: "channel-segment · process-ffmpeg-19 · ffmpeg 8.0",
    process: true,
  },
  {
    id: "ready",
    time: "10:42:10 PM",
    level: "info",
    source: "Android TV",
    message: "The player was ready for the upcoming handoff.",
    context: "Channel 7 · Living room TV",
    technical: "player.ready · session play_01K3D8J2",
  },
];

const tone = (level: PrototypeEvent["level"]) =>
  level === "error" ? "error" : level === "warn" ? "warn" : "ok";

const SourceIcon = ({ source }: { source: PrototypeEvent["source"] }) => {
  if (source === "Android TV") return <Tv aria-hidden className="size-4" />;
  if (source === "Web player") return <Smartphone aria-hidden className="size-4" />;
  if (source === "FFmpeg") return <Radio aria-hidden className="size-4" />;
  return <Server aria-hidden className="size-4" />;
};

const PrimaryTabs = () => (
  <nav className="flex gap-1 border-border border-b px-4 pt-2 sm:px-6" aria-label="Diagnostics views">
    <button
      type="button"
      aria-current="page"
      className="inline-flex items-center gap-2 rounded-t-md border border-border border-b-background bg-background px-3 py-2 font-medium text-foreground text-sm"
    >
      <Activity aria-hidden className="size-4" /> Logs
    </button>
    <button
      type="button"
      className="inline-flex items-center gap-2 rounded-t-md border border-transparent px-3 py-2 text-muted-foreground text-sm"
    >
      <HeartPulse aria-hidden className="size-4" /> Current Health
    </button>
  </nav>
);

const PrototypeHeader = ({ compact = false }: { compact?: boolean }) => (
  <>
    <PageHeader
      title="Diagnostics"
      description={
        compact ? "Find what happened in Loomarr." : "Live logs and current health for this Loomarr server."
      }
      actions={
        <Button variant="outline">
          <FileArchive aria-hidden /> Create support bundle
        </Button>
      }
    />
    <PrimaryTabs />
  </>
);

const SimpleToolbar = ({ large = false }: { large?: boolean }) => (
  <div
    className={`flex flex-wrap items-center gap-2 ${large ? "rounded-xl border border-border bg-card p-3" : ""}`}
  >
    <div className="relative min-w-56 flex-1">
      <Search aria-hidden className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
      <Input
        className={`${large ? "h-12 text-base" : ""} pl-9`}
        aria-label="Search logs"
        placeholder="Search logs"
      />
    </div>
    <Button variant="outline">
      Last hour <ChevronDown aria-hidden />
    </Button>
    <Button variant="outline">
      Errors + warnings <ChevronDown aria-hidden />
    </Button>
    <Button variant="ghost">
      <ListFilter aria-hidden /> More filters
    </Button>
  </div>
);

const LiveControl = () => (
  <div className="flex items-center gap-2 text-muted-foreground text-sm">
    <StatusDot tone="ok" label="Live" />
    <span>Live · updated just now</span>
    <Button variant="ghost" size="sm">
      <CirclePause aria-hidden /> Pause
    </Button>
  </div>
);

const EventIdentity = ({ event }: { event: PrototypeEvent }) => (
  <>
    <span className="inline-flex items-center gap-1.5 text-muted-foreground text-xs">
      <StatusDot tone={tone(event.level)} label={event.level} />
      <span className="capitalize">{event.level}</span>
      <span aria-hidden>·</span>
      <SourceIcon source={event.source} />
      {event.source}
    </span>
    <time className="font-mono text-muted-foreground text-xs">{event.time}</time>
  </>
);

const DetailPanel = ({ event }: { event: PrototypeEvent }) => (
  <aside className="space-y-5 rounded-lg border border-border bg-card p-4" aria-label="Selected log details">
    <div className="space-y-2">
      <EventIdentity event={event} />
      <h2 className="font-medium text-lg">{event.message}</h2>
      <p className="text-muted-foreground text-sm">{event.context}</p>
    </div>
    {event.process && (
      <div className="rounded-md border border-caution/30 bg-caution/5 p-3">
        <p className="font-medium text-sm">Related process output is available</p>
        <p className="mt-1 text-muted-foreground text-xs">See what FFmpeg reported around this failure.</p>
        <Button className="mt-3" size="sm" variant="outline">
          Open process output
        </Button>
      </div>
    )}
    <details className="rounded-md border border-border p-3">
      <summary className="cursor-pointer font-medium text-sm">Technical details</summary>
      <p className="mt-3 break-all font-mono text-muted-foreground text-xs">{event.technical}</p>
    </details>
  </aside>
);

const VariantA = () => {
  const [selected, setSelected] = useState(events[0]!);
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PrototypeHeader />
      <main className="min-h-0 flex-1 space-y-3 overflow-auto p-4 sm:p-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="font-medium text-lg">Logs</h2>
            <p className="text-muted-foreground text-sm">Most recent first</p>
          </div>
          <LiveControl />
        </div>
        <SimpleToolbar />
        <div className="grid min-h-[32rem] gap-3 xl:grid-cols-[minmax(0,1fr)_23rem]">
          <section className="overflow-hidden rounded-lg border border-border bg-card" aria-label="Live logs">
            {events.map((event) => (
              <button
                key={event.id}
                type="button"
                onClick={() => setSelected(event)}
                className={`grid w-full gap-2 border-border border-b px-4 py-3 text-left last:border-b-0 hover:bg-muted/30 sm:grid-cols-[8rem_minmax(0,1fr)_7rem] ${selected.id === event.id ? "bg-muted/30" : ""}`}
              >
                <span className="flex items-center gap-2 text-xs">
                  <StatusDot tone={tone(event.level)} label={event.level} />
                  <span className="capitalize">{event.level}</span>
                </span>
                <span>
                  <span className="block text-sm">{event.message}</span>
                  <span className="mt-1 block text-muted-foreground text-xs">
                    {event.source} · {event.context}
                  </span>
                </span>
                <time className="font-mono text-muted-foreground text-xs sm:text-right">{event.time}</time>
              </button>
            ))}
          </section>
          <DetailPanel event={selected} />
        </div>
      </main>
    </div>
  );
};

const VariantB = () => {
  const [selected, setSelected] = useState(events[0]!);
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PrototypeHeader compact />
      <main className="min-h-0 flex-1 overflow-auto p-4 sm:p-6">
        <div className="mb-4 grid gap-3 lg:grid-cols-[18rem_minmax(0,1fr)]">
          <aside className="space-y-3">
            <SimpleToolbar />
            <section className="rounded-lg border border-danger/30 bg-danger/5 p-4">
              <span className="text-danger text-xs">OPEN INCIDENT</span>
              <h2 className="mt-1 font-medium">Commercial handoff on Channel 7</h2>
              <p className="mt-1 text-muted-foreground text-sm">
                5 related logs across Loomarr, Web, Android TV, and FFmpeg.
              </p>
              <p className="mt-3 font-mono text-muted-foreground text-xs">10:42:10–10:42:19 PM</p>
            </section>
            <section className="rounded-lg border border-border bg-card p-4">
              <span className="text-muted-foreground text-xs">EARLIER</span>
              <h3 className="mt-1 font-medium text-sm">Library scan warning</h3>
              <p className="mt-1 text-muted-foreground text-xs">2 related logs · 9:18 PM</p>
            </section>
            <LiveControl />
          </aside>
          <section className="space-y-3" aria-label="Incident evidence">
            <div className="rounded-lg border border-border bg-card p-4">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div>
                  <p className="text-muted-foreground text-xs">LIKELY START</p>
                  <h2 className="font-medium text-lg">The next segment was not ready at handoff.</h2>
                </div>
                <Button variant="outline">
                  <Download aria-hidden /> Download these logs
                </Button>
              </div>
            </div>
            <ol className="relative ml-3 space-y-3 border-border border-l pl-5">
              {[...events].reverse().map((event) => (
                <li key={event.id} className="relative">
                  <span className="absolute top-4 -left-[1.63rem] size-3 rounded-full border-2 border-background bg-muted-foreground" />
                  <button
                    type="button"
                    onClick={() => setSelected(event)}
                    className="w-full rounded-lg border border-border bg-card p-4 text-left hover:bg-muted/30"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <EventIdentity event={event} />
                    </div>
                    <p className="mt-2 text-sm">{event.message}</p>
                    <p className="mt-1 text-muted-foreground text-xs">{event.context}</p>
                  </button>
                </li>
              ))}
            </ol>
            <DetailPanel event={selected} />
          </section>
        </div>
      </main>
    </div>
  );
};

const VariantC = () => {
  const [selected, setSelected] = useState<string>();
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PrototypeHeader compact />
      <main className="min-h-0 flex-1 overflow-auto p-4 sm:p-6">
        <div className="mx-auto max-w-4xl space-y-5">
          <div>
            <h2 className="font-semibold text-2xl">What happened?</h2>
            <p className="text-muted-foreground">Search recent Loomarr logs in plain language.</p>
          </div>
          <SimpleToolbar large />
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div className="rounded-lg border border-danger/30 bg-danger/5 p-3">
              <span className="text-muted-foreground text-xs">Errors</span>
              <strong className="mt-1 block text-2xl">1</strong>
            </div>
            <div className="rounded-lg border border-caution/30 bg-caution/5 p-3">
              <span className="text-muted-foreground text-xs">Warnings</span>
              <strong className="mt-1 block text-2xl">2</strong>
            </div>
            <div className="rounded-lg border border-border bg-card p-3">
              <span className="text-muted-foreground text-xs">Sources</span>
              <strong className="mt-1 block text-2xl">4</strong>
            </div>
            <div className="rounded-lg border border-border bg-card p-3">
              <span className="text-muted-foreground text-xs">Updates</span>
              <strong className="mt-1 flex items-center gap-2 text-sm">
                <StatusDot tone="ok" label="Live" /> Live
              </strong>
            </div>
          </div>
          <section aria-label="Recent logs">
            <div className="mb-2 flex items-center justify-between">
              <h3 className="font-medium">Recent logs</h3>
              <LiveControl />
            </div>
            <div className="space-y-2">
              {events.map((event) => {
                const open = selected === event.id;
                return (
                  <article key={event.id} className="overflow-hidden rounded-lg border border-border bg-card">
                    <button
                      type="button"
                      className="w-full p-4 text-left"
                      onClick={() => setSelected(open ? undefined : event.id)}
                      aria-expanded={open}
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <EventIdentity event={event} />
                      </div>
                      <p className="mt-2 font-medium">{event.message}</p>
                      <p className="mt-1 text-muted-foreground text-sm">{event.context}</p>
                    </button>
                    {open && (
                      <div className="border-border border-t p-4">
                        <DetailPanel event={event} />
                      </div>
                    )}
                  </article>
                );
              })}
            </div>
          </section>
        </div>
      </main>
    </div>
  );
};

const DiagnosticsPrototype = ({
  variant,
  onVariantChange,
  showSwitcher = false,
}: {
  variant: PrototypeVariant;
  onVariantChange?: (variant: PrototypeVariant) => void;
  showSwitcher?: boolean;
}) => (
  <>
    {variant === "A" && <VariantA />}
    {variant === "B" && <VariantB />}
    {variant === "C" && <VariantC />}
    {showSwitcher && onVariantChange && <PrototypeSwitcher variant={variant} onChange={onVariantChange} />}
  </>
);

export type { PrototypeVariant };
export { DiagnosticsPrototype };

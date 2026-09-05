import {
  ArrowRight,
  Check,
  CircleAlert,
  Clock3,
  Download,
  Film,
  Radio,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";

const stages = [
  {
    icon: Download,
    title: "Sources",
    count: "4 on",
    note: "Rights-aware acquisition",
    detail: "18 proposed or downloading",
    tone: "text-signal",
  },
  {
    icon: Film,
    title: "Prepare",
    count: "7",
    note: "Condition, split, identify",
    detail: "Models corroborate cuts",
    tone: "text-onair-300",
  },
  {
    icon: ShieldCheck,
    title: "Qualify",
    count: "12",
    note: "Role, safety, rights, playback",
    detail: "3 genuinely need you",
    tone: "text-caution",
  },
  {
    icon: Check,
    title: "Library",
    count: "284",
    note: "Verified playable children",
    detail: "Eligible for channel matching",
    tone: "text-lock",
  },
];

const VariantFlow = () => (
  <div className="flex flex-col gap-6 pb-20">
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <div className="flex items-center gap-2">
          <Badge variant="suggest">Prototype B</Badge>
          <Badge variant="lock">Recommended landing</Badge>
        </div>
        <h1 className="mt-2 font-semibold text-2xl">Filler is working. Three decisions need you.</h1>
        <p className="mt-1 max-w-3xl text-muted-foreground text-sm">
          Follow every item from acquisition to channel use without turning routine machine work into a human
          queue.
        </p>
      </div>
      <div className="flex items-center gap-2 rounded-full border border-lock/30 bg-lock/10 px-3 py-2 text-sm">
        <Radio className="size-4 text-lock" />
        <strong>12 of 12 channels covered</strong>
      </div>
    </header>

    <section className="rounded-xl border border-border bg-card p-5">
      <div className="grid gap-3 lg:grid-cols-[1fr_auto_1fr_auto_1fr_auto_1fr] lg:items-stretch">
        {stages.map((stage, index) => (
          <div key={stage.title} className="contents">
            <div className="rounded-lg border border-border bg-background p-4">
              <div className="flex items-start justify-between gap-3">
                <stage.icon className={`size-5 ${stage.tone}`} />
                <p className="font-mono text-2xl tabular-nums">{stage.count}</p>
              </div>
              <p className="mt-5 font-medium text-sm">{stage.title}</p>
              <Caption>{stage.note}</Caption>
              <p className="mt-3 text-muted-foreground text-xs">{stage.detail}</p>
            </div>
            {index < stages.length - 1 ? (
              <div className="hidden items-center lg:flex">
                <ArrowRight className="size-4 text-muted-foreground" />
              </div>
            ) : null}
          </div>
        ))}
      </div>
    </section>

    <div className="grid gap-5 xl:grid-cols-[1.35fr_1fr]">
      <section className="rounded-xl border border-border bg-card">
        <div className="flex items-center justify-between border-border border-b p-4">
          <div>
            <p className="font-medium text-sm">Moving automatically</p>
            <Caption>Visible progress; no clicks required</Caption>
          </div>
          <Badge variant="neutral">26 active</Badge>
        </div>
        <div className="divide-y divide-border">
          {[
            {
              name: "1990s cereal compilation",
              state: "Structure",
              detail: "Gemini and Seed agree on 7 child boundaries",
              progress: "72%",
            },
            {
              name: "Pacific Northwest station package",
              state: "Child qualification",
              detail: "5 roles verified · visual and spoken screening running",
              progress: "64%",
            },
            {
              name: "Automotive spots · batch 14",
              state: "Conditioning",
              detail: "Playback derivatives normalized and verified",
              progress: "42%",
            },
          ].map(({ name, state, detail, progress }) => (
            <div key={name} className="grid gap-3 p-4 sm:grid-cols-[1fr_140px] sm:items-center">
              <div>
                <p className="font-medium text-sm">{name}</p>
                <Caption>
                  {state} · {detail}
                </Caption>
              </div>
              <div>
                <div className="h-1.5 rounded-full bg-muted">
                  <div className="h-full rounded-full bg-signal" style={{ width: progress }} />
                </div>
                <p className="mt-1 text-right font-mono text-muted-foreground text-xs">{progress}</p>
              </div>
            </div>
          ))}
        </div>
        <div className="flex items-center gap-2 border-border border-t bg-muted/20 p-4 text-muted-foreground text-xs">
          <Sparkles className="size-4 text-signal" />
          Logos, companies, products, era, category, and audience evidence are added after each child is
          identified.
        </div>
      </section>

      <section className="rounded-xl border border-caution/35 bg-card">
        <div className="border-border border-b p-4">
          <div className="flex items-center gap-2">
            <CircleAlert className="size-4 text-caution" />
            <p className="font-medium text-sm">Your decisions</p>
            <Badge className="ml-auto" variant="caution">
              3
            </Badge>
          </div>
          <Caption>Only conflicts and exceptions reach this list</Caption>
        </div>
        <div className="divide-y divide-border">
          {[
            ["1", "Child role conflict", "Source evidence says PSA; video assessors say programme"],
            ["1", "Audience restriction", "Visual evidence requires an adult-audience decision"],
            ["1", "Current-use rights", "Grant expired; replacement evidence is available"],
          ].map(([count, title, detail]) => (
            <button
              type="button"
              key={title}
              className="flex w-full items-center gap-4 p-4 text-left hover:bg-muted/40"
            >
              <span className="grid size-9 place-items-center rounded-full bg-caution/10 font-mono text-caution">
                {count}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block font-medium text-sm">{title}</span>
                <span className="text-muted-foreground text-xs">{detail}</span>
              </span>
              <ArrowRight className="size-4 text-muted-foreground" />
            </button>
          ))}
        </div>
        <div className="space-y-3 border-border border-t p-4">
          <div className="flex items-center gap-2 text-muted-foreground text-xs">
            <Clock3 className="size-4" />
            Oldest decision has waited 46 minutes
          </div>
          <Button className="w-full">Review oldest exact child</Button>
        </div>
      </section>
    </div>
  </div>
);

export { VariantFlow };

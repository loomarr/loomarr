import { ArrowRight, Check, CircleAlert, Clock3, Download, Film, ShieldCheck } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";

const stages = [
  { icon: Download, title: "Acquire", count: 18, note: "Rights-aware source plans", tone: "text-signal" },
  { icon: Film, title: "Prepare", count: 7, note: "Split, condition, identify", tone: "text-onair-300" },
  {
    icon: ShieldCheck,
    title: "Certify",
    count: 12,
    note: "Safety, playback, audience, rights",
    tone: "text-caution",
  },
  { icon: Check, title: "Available", count: 284, note: "Eligible for channel matching", tone: "text-lock" },
];

const VariantFlow = () => (
  <div className="flex flex-col gap-6 pb-20">
    <header>
      <Badge variant="suggest">Prototype B</Badge>
      <h1 className="mt-2 font-semibold text-2xl">See the whole factory, not a pile of queues</h1>
      <p className="mt-1 max-w-3xl text-muted-foreground text-sm">
        A pipeline map for understanding throughput, bottlenecks, and exactly where human judgment enters.
      </p>
    </header>

    <section className="rounded-xl border border-border bg-card p-5">
      <div className="grid gap-3 lg:grid-cols-[1fr_auto_1fr_auto_1fr_auto_1fr] lg:items-stretch">
        {stages.map((stage, index) => (
          <div key={stage.title} className="contents">
            <div className="rounded-lg border border-border bg-background p-4">
              <stage.icon className={`size-5 ${stage.tone}`} />
              <p className="mt-5 font-mono text-3xl tabular-nums">{stage.count}</p>
              <p className="mt-1 font-medium text-sm">{stage.title}</p>
              <Caption>{stage.note}</Caption>
            </div>
            {index < stages.length - 1 && (
              <div className="hidden items-center lg:flex">
                <ArrowRight className="size-4 text-muted-foreground" />
              </div>
            )}
          </div>
        ))}
      </div>
      <div className="mt-4 flex items-center gap-2 rounded-lg bg-caution/10 px-4 py-3 text-sm">
        <CircleAlert className="size-4 text-caution" />
        <strong>Certification is the bottleneck.</strong>
        <span className="text-muted-foreground">
          8 clips have evidence conflicts; 4 are ready for a person.
        </span>
      </div>
    </section>

    <div className="grid gap-5 xl:grid-cols-[1.35fr_1fr]">
      <section className="rounded-xl border border-border bg-card">
        <div className="flex items-center justify-between border-border border-b p-4">
          <div>
            <p className="font-medium text-sm">Work moving now</p>
            <Caption>Machine-owned; no clicks required</Caption>
          </div>
          <Button size="sm" variant="outline">
            View all activity
          </Button>
        </div>
        <div className="divide-y divide-border">
          {[
            ["1990s cereal compilation", "Splitting", "38 of 44 boundaries analysed", "86%"],
            ["KCPQ station package", "Screening", "Visual and written evidence complete", "64%"],
            ["Automotive spots · batch 14", "Conditioning", "Normalising playback derivatives", "42%"],
          ].map(([name, state, detail, progress]) => (
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
      </section>

      <section className="rounded-xl border border-border bg-card">
        <div className="border-border border-b p-4">
          <p className="font-medium text-sm">Your decisions</p>
          <Caption>Only work the machine cannot settle</Caption>
        </div>
        <div className="divide-y divide-border">
          {[
            ["4", "Audience suitability", "Two spoken, two visual conflicts"],
            ["5", "Rights evidence", "Current-use grant is incomplete"],
            ["3", "Compilation cuts", "Boundary confidence below policy"],
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
        <div className="flex items-center gap-2 border-border border-t p-4 text-muted-foreground text-xs">
          <Clock3 className="size-4" />
          Oldest decision has waited 2 days
        </div>
      </section>
    </div>
  </div>
);

export { VariantFlow };

import type { LucideIcon } from "lucide-react";
import { Check, Clock3, Eye, FileText, Film, ShieldCheck, Volume2, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";

const queue = [
  ["Coca-Cola · Summer refresh", "0:30", "Needs review"],
  ["KCPQ station ident", "0:10", "Ready after this"],
  ["Mattel Hot Wheels", "0:29", "Rights hold"],
  ["Local car dealership", "0:31", "Preparing"],
];

const axis: [LucideIcon, string, "Pass" | "Review", string][] = [
  [Eye, "Visual", "Pass", "No nudity, gore, or unsafe imagery detected"],
  [Volume2, "Spoken", "Review", "One degrading-language observation at 00:18"],
  [FileText, "Written", "Pass", "On-screen copy screened across sampled frames"],
  [ShieldCheck, "Rights", "Pass", "Current-use grant verified for this exact source"],
  [Film, "Playback", "Pass", "Complete decode, seek, loudness, cadence, and exact bytes verified"],
];

const VariantReview = () => (
  <div className="flex flex-col gap-5 pb-20">
    <header className="flex flex-wrap items-end justify-between gap-3">
      <div>
        <Badge variant="suggest">Prototype A</Badge>
        <h1 className="mt-2 font-semibold text-2xl">Make one defensible decision at a time</h1>
        <p className="mt-1 max-w-2xl text-muted-foreground text-sm">
          A focused review desk: queue on the left, exact playback in the middle, release evidence on the
          right.
        </p>
      </div>
      <Caption>Read-only representative data</Caption>
    </header>

    <div className="grid min-h-[620px] gap-4 xl:grid-cols-[260px_minmax(420px,1fr)_330px]">
      <aside className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="border-border border-b p-4">
          <p className="font-medium text-sm">Needs a decision</p>
          <Caption>12 clips · oldest first</Caption>
        </div>
        <ol className="divide-y divide-border">
          {queue.map(([name, duration, state], index) => (
            <li key={name} className={index === 0 ? "bg-signal/10 p-4" : "p-4"}>
              <div className="flex items-start gap-3">
                <span className="font-mono text-muted-foreground text-xs">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium text-sm">{name}</p>
                  <p className="mt-1 text-muted-foreground text-xs">
                    {duration} · {state}
                  </p>
                </div>
              </div>
            </li>
          ))}
        </ol>
      </aside>

      <main className="flex min-w-0 flex-col overflow-hidden rounded-xl border border-border bg-card">
        <div className="flex items-start justify-between gap-3 border-border border-b p-4">
          <div>
            <p className="font-semibold">Coca-Cola · Summer refresh</p>
            <Caption>Commercial · 1994 · General audience · beverages</Caption>
          </div>
          <Badge variant="neutral">Tag confidence 92</Badge>
        </div>
        <div className="grid flex-1 place-items-center bg-static-950 p-8 text-static-300">
          <div className="text-center">
            <div className="mx-auto grid size-16 place-items-center rounded-full border border-static-700">
              <span className="ml-1 text-2xl">▶</span>
            </div>
            <p className="mt-4 font-medium text-sm text-static-100">Watch the complete 30-second clip</p>
            <p className="mt-1 text-static-400 text-xs">
              A decision unlocks only after playback actually starts
            </p>
          </div>
        </div>
        <div className="border-border border-t p-4">
          <div className="flex items-center gap-2 text-xs">
            <Clock3 className="size-4 text-muted-foreground" />
            <span>00:00</span>
            <div className="h-1 flex-1 rounded-full bg-muted" />
            <span>00:30</span>
          </div>
        </div>
      </main>

      <aside className="flex flex-col rounded-xl border border-border bg-card">
        <div className="border-border border-b p-4">
          <div className="flex items-center gap-2">
            <p className="font-medium text-sm">Release evidence</p>
            <Badge className="ml-auto" variant="caution">
              Airworthiness hold
            </Badge>
          </div>
          <Caption>Five required axes · exact playback bytes · screened today</Caption>
        </div>
        <div className="flex-1 divide-y divide-border">
          {axis.map(([Icon, label, result, detail]) => (
            <div key={String(label)} className="p-4">
              <div className="flex items-center gap-2">
                <Icon className="size-4 text-muted-foreground" />
                <span className="font-medium text-sm">{String(label)}</span>
                <Badge className="ml-auto" variant={result === "Pass" ? "lock" : "caution"}>
                  {String(result)}
                </Badge>
              </div>
              <p className="mt-2 text-muted-foreground text-xs leading-relaxed">{String(detail)}</p>
            </div>
          ))}
        </div>
        <div className="space-y-2 border-border border-t p-4">
          <p className="text-muted-foreground text-xs">
            Spoken evidence prevents admission. Correct the classification or reject it.
          </p>
          <Button className="w-full" variant="outline" disabled>
            <Check className="size-4" /> Admit to library
          </Button>
          <Button className="w-full" variant="outline">
            <X className="size-4" /> Reject clip
          </Button>
        </div>
      </aside>
    </div>
  </div>
);

export { VariantReview };

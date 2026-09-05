import type { LucideIcon } from "lucide-react";
import { Check, Clock3, Eye, FileText, Film, GitBranch, ShieldCheck, Sparkles, Volume2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Caption } from "@/components/ui/caption";

const queue = [
  ["Community-service child", "Role conflict", "Source evidence available"],
  ["Swimwear spot", "Audience restriction", "Visual evidence"],
  ["Radio promotion", "Spoken rejection", "Prohibited language"],
  ["Local dealership", "Rights exception", "Grant expired"],
];

const evidence: [LucideIcon, string, "Pass" | "Review", string][] = [
  [
    GitBranch,
    "Structure",
    "Pass",
    "Gemini and Seed independently agree on this child boundary within 434 ms",
  ],
  [
    Sparkles,
    "Exact role",
    "Review",
    "Source authority says PSA; direct-video evidence looks like programme content",
  ],
  [Eye, "Visual", "Pass", "No prohibited visual observation across the complete child"],
  [Volume2, "Spoken", "Pass", "Complete spoken coverage; no restricted-language observation"],
  [FileText, "Written", "Pass", "On-screen text and title cards screened"],
  [ShieldCheck, "Rights", "Pass", "Current-use grant binds this source master and derivative"],
  [Film, "Playback", "Pass", "Decode, seek, cadence, loudness, duration, and exact bytes verified"],
];

const VariantReview = () => (
  <div className="flex flex-col gap-5 pb-20">
    <header className="flex flex-wrap items-end justify-between gap-3">
      <div>
        <Badge variant="suggest">Prototype A</Badge>
        <h1 className="mt-2 font-semibold text-2xl">
          Resolve one real exception, with the evidence beside it
        </h1>
        <p className="mt-1 max-w-3xl text-muted-foreground text-sm">
          No blind audit and no confidence score: the machine asks only when authoritative evidence conflicts.
        </p>
      </div>
      <Caption>Read-only representative data · exact child bytes</Caption>
    </header>

    <div className="grid min-h-[690px] gap-4 xl:grid-cols-[260px_minmax(440px,1fr)_360px]">
      <aside className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="border-border border-b p-4">
          <p className="font-medium text-sm">Needs you</p>
          <Caption>4 genuine exceptions · machine-ranked</Caption>
        </div>
        <ol className="divide-y divide-border">
          {queue.map(([name, state, detail], index) => (
            <li key={name} className={index === 0 ? "bg-caution/10 p-4" : "p-4"}>
              <div className="flex items-start gap-3">
                <span className="font-mono text-muted-foreground text-xs">
                  {String(index + 1).padStart(2, "0")}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium text-sm">{name}</p>
                  <p className="mt-1 text-caution text-xs">{state}</p>
                  <p className="mt-1 text-muted-foreground text-xs">{detail}</p>
                </div>
              </div>
            </li>
          ))}
        </ol>
        <div className="border-border border-t p-4 text-muted-foreground text-xs">
          Routine acquisition, splitting, screening, and enrichment continue without you.
        </div>
      </aside>

      <main className="flex min-w-0 flex-col overflow-hidden rounded-xl border border-border bg-card">
        <div className="flex flex-wrap items-start justify-between gap-3 border-border border-b p-4">
          <div>
            <p className="font-semibold">Community-service compilation · child 01</p>
            <Caption>Exact conditioned child · 00:00–02:00 · held and unclassified</Caption>
          </div>
          <Badge variant="caution">Role unresolved</Badge>
        </div>

        <div className="grid min-h-80 flex-1 place-items-center bg-static-950 p-8 text-static-300">
          <div className="text-center">
            <div className="mx-auto grid size-16 place-items-center rounded-full border border-static-700">
              <span className="ml-1 text-2xl">▶</span>
            </div>
            <p className="mt-4 font-medium text-sm text-static-100">Play the exact child being classified</p>
            <p className="mt-1 text-static-400 text-xs">
              Not the source reel and not a model-selected excerpt
            </p>
          </div>
        </div>

        <div className="space-y-3 border-border border-t p-4">
          <div className="flex items-center gap-2 text-xs">
            <Clock3 className="size-4 text-muted-foreground" />
            <span>00:00</span>
            <div className="h-1 flex-1 rounded-full bg-muted" />
            <span>02:00</span>
          </div>
          <div>
            <div className="flex h-10 overflow-hidden rounded-md border border-border text-xs">
              <div className="flex w-[81%] items-center justify-between bg-caution/15 px-3">
                <strong>Child 01 · selected</strong>
                <span>02:00</span>
              </div>
              <div className="flex w-[19%] items-center justify-center border-border border-l bg-muted/40 px-2">
                Child 02
              </div>
            </div>
            <p className="mt-2 text-muted-foreground text-xs">
              Two independent video families agree on the cut. Exact role is decided from this final child,
              not inherited from the parent compilation.
            </p>
          </div>
        </div>

        <section
          className="grid gap-px border-border border-t bg-border sm:grid-cols-3"
          aria-label="Identity evidence"
        >
          {[
            ["Organization", "Richmond Senior Center"],
            ["Category", "Community service"],
            ["Era", "1990s source"],
          ].map(([label, value]) => (
            <div key={label} className="bg-card p-3">
              <p className="text-muted-foreground text-xs">{label}</p>
              <p className="mt-1 font-medium text-sm">{value}</p>
            </div>
          ))}
        </section>
      </main>

      <aside className="flex flex-col rounded-xl border border-border bg-card">
        <div className="border-border border-b p-4">
          <div className="flex items-center gap-2">
            <p className="font-medium text-sm">Why this is held</p>
            <Badge className="ml-auto" variant="caution">
              1 unresolved axis
            </Badge>
          </div>
          <Caption>Independent claims stay separate; there is no averaged “safe” score</Caption>
        </div>
        <div className="flex-1 divide-y divide-border">
          {evidence.map(([Icon, label, result, detail]) => (
            <div key={label} className="p-3.5">
              <div className="flex items-center gap-2">
                <Icon className="size-4 text-muted-foreground" />
                <span className="font-medium text-sm">{label}</span>
                <Badge className="ml-auto" variant={result === "Pass" ? "lock" : "caution"}>
                  {result}
                </Badge>
              </div>
              <p className="mt-1.5 text-muted-foreground text-xs leading-relaxed">{detail}</p>
            </div>
          ))}
        </div>
        <div className="space-y-2 border-border border-t p-4">
          <div className="rounded-md bg-caution/10 p-3 text-xs">
            <strong>Question:</strong> Is this final child a PSA or programme content? It remains
            unschedulable until the role authority is resolved.
          </div>
          <Button className="w-full">
            <Check className="size-4" /> Classify as PSA
          </Button>
          <div className="grid grid-cols-2 gap-2">
            <Button variant="outline">Programme</Button>
            <Button variant="ghost">Keep held</Button>
          </div>
        </div>
      </aside>
    </div>
  </div>
);

export { VariantReview };

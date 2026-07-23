import type { DiscoverModelView } from "@loomarr/api";
import { formatCompactCount, formatGiB, formatPercentPoints } from "@loomarr/core";
import { Download, Loader2 } from "lucide-react";
import { Badge, Button } from "@/components/ui";
import { cn } from "@/lib";
import type { ModelDiscoverProps } from "./model-discover.type";

// The §8.1 "download a model" surface. Ollama ships no API for enumerating pullable
// models, so Loomarr browses Hugging Face's GGUF catalog live, sizes each repo against
// this machine's VRAM (HF exposes real per-quant sizes before download), and the BE
// returns only COMPATIBLE models, ranked best-first. This component just renders that
// ranked list with a Download per row — no search box: it's the compatible set for the
// hardware. The pull + SSE progress live in the parent, like ModelPicker.
//
// Tool-capability is UNKNOWN here on purpose — it's confirmed after the model is pulled
// and re-probed, at which point the model shows up in the installed picker above.
const FIT_LABEL: Record<string, { text: string; variant: "lock" | "caution" | "onair" }> = {
  fits: { text: "fits", variant: "lock" },
  tight: { text: "tight", variant: "caution" },
  wont_fit: { text: "won't fit", variant: "onair" },
};

const ModelDiscover = ({
  results,
  loading = false,
  sourceError = false,
  pulling,
  onPull,
  className,
}: ModelDiscoverProps) => (
  <div className={cn("flex flex-col gap-3", className)}>
    <div>
      <p className="font-medium text-sm">Download a model</p>
      <p className="mt-0.5 text-muted-foreground text-sm">
        Models that fit your hardware and can generate lineups, most popular first.
      </p>
    </div>

    {loading && (
      <p className="flex items-center gap-2 text-muted-foreground text-sm">
        <Loader2 className="size-4 animate-spin" aria-hidden />
        Finding compatible models…
      </p>
    )}

    {sourceError && (
      <p className="text-muted-foreground text-sm">
        Couldn't reach the model catalog.{" "}
        <a
          href="https://huggingface.co/models?library=gguf&sort=downloads"
          target="_blank"
          rel="noreferrer"
          className="text-signal underline underline-offset-2"
        >
          Browse on huggingface.co ↗
        </a>
      </p>
    )}

    {results && results.length === 0 && !loading && !sourceError && (
      <p className="text-muted-foreground text-sm">
        No compatible models found right now. Try again shortly.
      </p>
    )}

    {results && results.length > 0 && (
      <ul className="flex flex-col gap-2">
        {results.map((m: DiscoverModelView) => {
          const fit = FIT_LABEL[m.fit] ?? { text: m.fit, variant: "caution" as const };
          const isPulling = pulling?.tag === m.pullRef;
          const progress = isPulling ? pulling?.percent : undefined;
          return (
            <li
              key={m.id}
              className="flex items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5"
            >
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium text-sm" title={m.id}>
                  {m.label}
                </p>
                <p className="mt-1 flex flex-wrap items-center gap-2 text-xs">
                  <Badge variant={fit.variant}>{fit.text}</Badge>
                  <span className="text-static-400">
                    {m.quant} · ~{formatGiB(m.sizeGiB)}
                  </span>
                  <span className="text-static-400">{formatCompactCount(m.downloads)} downloads</span>
                  {isPulling && (
                    <span className="text-tune">
                      downloading{progress !== undefined ? ` ${formatPercentPoints(progress)}` : "…"}
                    </span>
                  )}
                </p>
              </div>
              <Button variant="outline" size="sm" disabled={isPulling} onClick={() => onPull(m.pullRef)}>
                {isPulling ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                ) : (
                  <Download className="size-4" aria-hidden />
                )}
                {isPulling ? "Downloading…" : "Download"}
              </Button>
            </li>
          );
        })}
      </ul>
    )}
  </div>
);

export { ModelDiscover };

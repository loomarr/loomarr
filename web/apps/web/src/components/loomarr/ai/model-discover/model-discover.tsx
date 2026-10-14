import type { DiscoverModelView } from "@loomarr/api/models/discoverModelView";
import { formatGiB, formatPercentPoints } from "@loomarr/core/format";
import { Download, Loader2 } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ModelDiscoverProps } from "./model-discover.type";

// The §8.1 "download a model" surface — the QUIET counterpart to the installed picker above
// it. Ollama ships no API for enumerating pullable models, so the BE browses Hugging Face's
// GGUF catalog live, sizes each repo against this machine's VRAM, filters out what's already
// installed, and returns a list RANKED FOR LOOMARR (fit → family reliability → size,
// popularity only as a last tiebreak) with one `recommended` pick and a plain-English
// `role`/`note` per model. Downloading is the EXCEPTION path — most users pick an installed
// model — so this section is understated: no loud hero, a small "suggested" tag on the top
// pick, muted "Get" actions. No HF jargon (quant, download counts) reaches the row.
//
// Tool-capability is confirmed only after pull; a downloaded model then appears in the
// installed picker above, which is where you actually switch to it.

// How many models to show before "show more".
const DEFAULT_VISIBLE = 4;

const ModelDiscover = ({
  results,
  loading = false,
  sourceError = false,
  pulling,
  onPull,
  className,
}: ModelDiscoverProps) => {
  const [expanded, setExpanded] = useState(false);
  const total = results?.length ?? 0;
  const visible = expanded ? results : results?.slice(0, DEFAULT_VISIBLE);
  const hiddenCount = total - (visible?.length ?? 0);

  return (
    <div className={cn("flex flex-col gap-2.5", className)}>
      <div>
        <p className="font-medium text-muted-foreground text-sm">Need a different model?</p>
        <p className="mt-0.5 text-muted-foreground text-xs">
          These fit your hardware and can generate lineups. Download one to add it to the list above.
        </p>
      </div>

      {loading && (
        <p className="flex items-center gap-2 text-muted-foreground text-xs">
          <Loader2 className="size-3.5 animate-spin" aria-hidden />
          Finding compatible models…
        </p>
      )}

      {sourceError && (
        <p className="text-muted-foreground text-xs">
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
        <p className="text-muted-foreground text-xs">No other compatible models found right now.</p>
      )}

      {visible && visible.length > 0 && (
        <ul className="flex flex-col divide-y divide-border/60 rounded-md border border-border/60">
          {visible.map((m: DiscoverModelView) => {
            const isPulling = pulling?.tag === m.pullRef;
            const progress = isPulling ? pulling?.percent : undefined;
            return (
              <li key={m.id} className="flex items-center gap-3 px-3 py-2">
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-2">
                    <span className="truncate font-medium text-sm" title={m.id}>
                      {m.label}
                    </span>
                    {m.recommended && (
                      <Badge variant="tune" className="shrink-0">
                        suggested
                      </Badge>
                    )}
                  </p>
                  {/* The note carries the fit in plain words ("fits comfortably" /
                      "fits, but tight on VRAM"), so no separate fit badge is needed. */}
                  <p className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-muted-foreground text-xs">
                    <span>{m.note}</span>
                    <span aria-hidden className="text-static-400">
                      ·
                    </span>
                    <span className="text-static-400">~{formatGiB(m.sizeGiB)}</span>
                    {isPulling && (
                      <span className="text-tune">
                        downloading
                        {progress !== undefined ? ` ${formatPercentPoints(progress)}` : "…"}
                      </span>
                    )}
                  </p>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={isPulling}
                  onClick={() => onPull(m.pullRef)}
                  className="shrink-0 text-muted-foreground"
                >
                  {isPulling ? (
                    <Loader2 className="size-4 animate-spin" aria-hidden />
                  ) : (
                    <Download className="size-4" aria-hidden />
                  )}
                  {isPulling ? "Getting…" : "Get"}
                </Button>
              </li>
            );
          })}
        </ul>
      )}

      {!expanded && hiddenCount > 0 && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start text-muted-foreground text-xs"
          onClick={() => setExpanded(true)}
        >
          Show {hiddenCount} more
        </Button>
      )}
      {expanded && total > DEFAULT_VISIBLE && (
        <Button
          variant="ghost"
          size="sm"
          className="self-start text-muted-foreground text-xs"
          onClick={() => setExpanded(false)}
        >
          Show fewer
        </Button>
      )}
    </div>
  );
};

export { ModelDiscover };

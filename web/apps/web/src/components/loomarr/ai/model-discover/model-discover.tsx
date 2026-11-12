import type { DiscoverModelView } from "@loomarr/api/models/discoverModelView";
import { formatGiB, formatPercentPoints } from "@loomarr/core/format";
import { Action, ActivityIndicator, Badge, Stack } from "@loomarr/design-system";
import { useState } from "react";
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
        <p className="text-muted-foreground text-sm font-medium">Need a different model?</p>
        <p className="text-muted-foreground mt-0.5 text-xs">
          These fit your hardware and can generate lineups. Download one to add it to the list above.
        </p>
      </div>

      {loading && (
        <Stack
          align="center"
          className="text-muted-foreground text-xs"
          direction="row"
          gap="inline"
          render="p"
        >
          <ActivityIndicator decorative size="compact" tone="inherit" />
          Finding compatible models…
        </Stack>
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
        <Stack className="divide-border/60 border-border/60 divide-y rounded-md border" render="ul">
          {visible.map((m: DiscoverModelView) => {
            const isPulling = pulling?.tag === m.pullRef;
            const progress = isPulling ? pulling?.percent : undefined;
            return (
              <Stack
                align="center"
                className="px-3 py-2"
                direction="row"
                gap="control"
                key={m.id}
                render="li"
              >
                <div className="min-w-0 flex-1">
                  <Stack align="center" direction="row" gap="inline" render="p">
                    <span className="truncate text-sm font-medium" title={m.id}>
                      {m.label}
                    </span>
                    {m.recommended && (
                      <Badge shape="square" tone="tune" flexShrink={0}>
                        suggested
                      </Badge>
                    )}
                  </Stack>
                  {/* The note carries the fit in plain words ("fits comfortably" /
                      "fits, but tight on VRAM"), so no separate fit badge is needed. */}
                  <p className="text-muted-foreground mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
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
                <div className="shrink-0">
                  <Action
                    disabled={isPulling}
                    icon={isPulling ? "loading" : "download"}
                    onPress={() => onPull(m.pullRef)}
                    variant="ghost"
                  >
                    {isPulling ? "Getting…" : "Get"}
                  </Action>
                </div>
              </Stack>
            );
          })}
        </Stack>
      )}

      {!expanded && hiddenCount > 0 && (
        <Action onPress={() => setExpanded(true)} style={{ alignSelf: "flex-start" }} variant="ghost">
          {`Show ${hiddenCount} more`}
        </Action>
      )}
      {expanded && total > DEFAULT_VISIBLE && (
        <Action onPress={() => setExpanded(false)} style={{ alignSelf: "flex-start" }} variant="ghost">
          Show fewer
        </Action>
      )}
    </div>
  );
};

export { ModelDiscover };

import type { DiscoverModelView } from "@loomarr/api/models/discoverModelView";
import { formatGiB, formatPercentPoints } from "@loomarr/core/format";
import { Action, ActivityIndicator, Badge, Stack, Text, TextLink } from "@loomarr/design-system";
import { useState } from "react";
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
}: ModelDiscoverProps) => {
  const [expanded, setExpanded] = useState(false);
  const total = results?.length ?? 0;
  const visible = expanded ? results : results?.slice(0, DEFAULT_VISIBLE);
  const hiddenCount = total - (visible?.length ?? 0);

  return (
    <Stack gap="2.5">
      <div>
        <Text as="p" textRole="cardLabel" tone="secondary">
          Need a different model?
        </Text>
        <Stack marginTop={2}>
          <Text as="p" textRole="hint" tone="secondary">
            These fit your hardware and can generate lineups. Download one to add it to the list above.
          </Text>
        </Stack>
      </div>

      {loading && (
        <Stack align="center" direction="row" gap="inline" render="p">
          <ActivityIndicator decorative size="compact" tone="secondary" />
          <Text as="span" textRole="hint" tone="secondary">
            Finding compatible models…
          </Text>
        </Stack>
      )}

      {sourceError && (
        <Text as="p" textRole="hint" tone="secondary">
          Couldn't reach the model catalog.{" "}
          <TextLink external href="https://huggingface.co/models?library=gguf&sort=downloads">
            Browse on huggingface.co
          </TextLink>
        </Text>
      )}

      {results && results.length === 0 && !loading && !sourceError && (
        <Text as="p" textRole="hint" tone="secondary">
          No other compatible models found right now.
        </Text>
      )}

      {visible && visible.length > 0 && (
        <Stack borderColor="borderDecorative" borderRadius={6} borderWidth={1} divided render="ul">
          {visible.map((m: DiscoverModelView) => {
            const isPulling = pulling?.tag === m.pullRef;
            const progress = isPulling ? pulling?.percent : undefined;
            return (
              <Stack
                align="center"
                direction="row"
                gap="control"
                key={m.id}
                paddingHorizontal={12}
                paddingVertical={8}
                render="li"
              >
                <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
                  <Stack align="center" direction="row" gap="inline" render="p">
                    <Stack flexShrink={1} minWidth={0} render={<span title={m.id} />}>
                      <Text as="span" numberOfLines={1} textRole="cardLabel" tone="primary">
                        {m.label}
                      </Text>
                    </Stack>
                    {m.recommended && (
                      <Badge shape="square" tone="tune" flexShrink={0}>
                        suggested
                      </Badge>
                    )}
                  </Stack>
                  {/* The note carries the fit in plain words ("fits comfortably" /
                      "fits, but tight on VRAM"), so no separate fit badge is needed. */}
                  <Stack
                    align="center"
                    columnGap="2"
                    direction="row"
                    marginTop={2}
                    render="p"
                    rowGap="0.5"
                    wrap
                  >
                    <Text as="span" textRole="hint" tone="secondary">
                      {m.note}
                    </Text>
                    <Text aria-hidden as="span" textRole="hint" tone="secondary">
                      ·
                    </Text>
                    <Text as="span" textRole="hint" tone="secondary">
                      ~{formatGiB(m.sizeGiB)}
                    </Text>
                    {isPulling && (
                      <Text as="span" textRole="hint" tone="info">
                        downloading
                        {progress !== undefined ? ` ${formatPercentPoints(progress)}` : "…"}
                      </Text>
                    )}
                  </Stack>
                </Stack>
                <Stack flexShrink={0}>
                  <Action
                    disabled={isPulling}
                    icon={isPulling ? "loading" : "download"}
                    onPress={() => onPull(m.pullRef)}
                    variant="ghost"
                  >
                    {isPulling ? "Getting…" : "Get"}
                  </Action>
                </Stack>
              </Stack>
            );
          })}
        </Stack>
      )}

      {!expanded && hiddenCount > 0 && (
        <Action onPress={() => setExpanded(true)} alignSelf="flex-start" variant="ghost">
          {`Show ${hiddenCount} more`}
        </Action>
      )}
      {expanded && total > DEFAULT_VISIBLE && (
        <Action onPress={() => setExpanded(false)} alignSelf="flex-start" variant="ghost">
          Show fewer
        </Action>
      )}
    </Stack>
  );
};

export { ModelDiscover };

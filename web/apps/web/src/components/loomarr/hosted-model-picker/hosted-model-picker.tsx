import type { HostedModelView, HostedProviderView } from "@loomarr/api";
import { Check } from "lucide-react";
import { Badge, Button } from "@/components/ui";
import { cn } from "@/lib";
import type { HostedModelPickerProps } from "./hosted-model-picker.type";

// The §8.1 hosted picker (OpenAI-compatible). The BE curates two hosted providers —
// OpenRouter and a Custom endpoint — and for one with a stored key fetches its LIVE
// model list, ranked for grounding (tool-capable + cheap first). This component renders
// that: pick a model → it hot-swaps the suggester (no restart). Before a key is set, it
// shows where to get one rather than an empty list. Keys live in the settings form + Test
// above; this surface never handles the key itself.
const HostedModelPicker = ({
  providers,
  activeModel,
  onSelect,
  busy = false,
  className,
}: HostedModelPickerProps) => (
  <div className={cn("flex flex-col gap-4", className)}>
    {providers.map((hp: HostedProviderView) => {
      const models = hp.models ?? [];
      return (
        <div key={hp.key} className="flex flex-col gap-2">
          <div className="flex items-baseline justify-between gap-2">
            <p className="font-medium text-sm">
              {hp.label}
              {hp.active && <span className="ml-2 text-muted-foreground text-xs">(in use)</span>}
            </p>
            {hp.modelsLive ? (
              <span className="text-static-400 text-xs">live models</span>
            ) : (
              <span className="text-static-400 text-xs">suggested</span>
            )}
          </div>

          {!hp.keyConfigured ? (
            // No key yet: point the operator at where to get one. The key is entered in
            // the settings form above (llm.api_key), not here.
            <p className="text-muted-foreground text-sm">
              Add this provider's URL + key above and press Test to list its models.
              {hp.keysUrl && (
                <>
                  {" "}
                  <a
                    href={hp.keysUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="text-signal underline underline-offset-2"
                  >
                    Get a key ↗
                  </a>
                </>
              )}
            </p>
          ) : models.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No models returned — check the key and base URL above, then Test again.
            </p>
          ) : (
            <ul className="flex flex-col gap-2">
              {models.map((m: HostedModelView) => {
                const isActive = hp.active && m.id === activeModel;
                return (
                  <li
                    key={m.id}
                    className={cn(
                      "flex items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5",
                      isActive && "border-signal-tint-30",
                    )}
                  >
                    <div className="min-w-0 flex-1">
                      <p className="flex flex-wrap items-center gap-2 font-medium text-sm">
                        {m.label}
                        <span className="font-mono text-static-400 text-xs">{m.id}</span>
                        {m.recommended && <Badge variant="signal">recommended</Badge>}
                        {m.tools && <Badge variant="lock">tools</Badge>}
                        {isActive && <Badge variant="lock">active</Badge>}
                      </p>
                      {m.why && <p className="mt-0.5 text-muted-foreground text-sm">{m.why}</p>}
                    </div>
                    <Button
                      variant={isActive ? "outline" : "default"}
                      size="sm"
                      disabled={busy || isActive}
                      onClick={() => onSelect({ provider: hp.key, model: m.id, baseUrl: hp.baseUrl })}
                    >
                      {isActive ? <Check className="size-4" aria-hidden /> : null}
                      {isActive ? "In use" : "Use this"}
                    </Button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      );
    })}
  </div>
);

export { HostedModelPicker };

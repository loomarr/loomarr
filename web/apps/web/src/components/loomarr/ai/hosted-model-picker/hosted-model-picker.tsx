import type { HostedModelView } from "@loomarr/api/models/hostedModelView";
import type { HostedProviderView } from "@loomarr/api/models/hostedProviderView";
import { Check, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { HostedModelPickerProps } from "./hosted-model-picker.type";

// The §8.1 hosted picker (OpenAI-compatible). The BE curates two hosted providers —
// OpenRouter and a Custom endpoint — and for one with a stored key fetches its LIVE
// model list, ranked for grounding (verified tool-capable + cheap first, unknown capability
// explicit). This component renders that: pick a model → it hot-swaps the suggester. Before a key is set, it
// shows where to get one rather than an empty list. Keys live in the settings form + Test
// above; this surface never handles the key itself.
const HostedModelPicker = ({
  providers,
  activeModel,
  onSelect,
  onVerify,
  verifying,
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

          {!hp.keyConfigured && (
            // Keep the safe fallback visible below this instruction: it answers which
            // model Loomarr recommends before credentials can unlock the live catalog.
            <p className="text-muted-foreground text-sm">
              {hp.key === "openrouter"
                ? "Add your OpenRouter key above and save to use the suggested model and load live options."
                : "Add this provider's URL + key above and save to load its models."}
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
          )}

          {hp.keyConfigured && models.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              No models returned. Check the key and base URL above, then Test again.
            </p>
          ) : models.length > 0 ? (
            <ul className="flex flex-col gap-2">
              {models.map((m: HostedModelView) => {
                const isActive = hp.active && m.id === activeModel;
                const toolCapability =
                  m.toolCapability === "verified" || m.toolCapability === "unsupported"
                    ? m.toolCapability
                    : "unverified";
                const toolsUnverified = toolCapability === "unverified";
                const toolsUnsupported = toolCapability === "unsupported";
                const isVerifying = verifying?.provider === hp.key && verifying.model === m.id;
                const action = isActive
                  ? "In use"
                  : !hp.keyConfigured
                    ? "Add key first"
                    : toolsUnsupported
                      ? "Can't use"
                      : "Use this";
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
                        <Badge
                          variant={
                            toolCapability === "verified"
                              ? "lock"
                              : toolCapability === "unsupported"
                                ? "onair"
                                : "caution"
                          }
                        >
                          {toolCapability === "verified"
                            ? "Tools verified"
                            : toolCapability === "unsupported"
                              ? "Tools unsupported"
                              : "Tools unverified"}
                        </Badge>
                        {isActive && <Badge variant="lock">active</Badge>}
                      </p>
                      {m.why && <p className="mt-0.5 text-muted-foreground text-sm">{m.why}</p>}
                      {toolsUnsupported && !m.why && (
                        <p className="mt-0.5 text-muted-foreground text-sm">
                          This model cannot call tools, so Loomarr cannot ground its suggestions safely.
                        </p>
                      )}
                      {toolsUnverified && (
                        <p className="mt-1 text-muted-foreground text-xs">
                          Verification makes one small inference call. It proves tool calling only and does
                          not certify curation quality.
                        </p>
                      )}
                    </div>
                    {toolsUnverified && hp.keyConfigured ? (
                      <Button
                        variant="suggest"
                        size="sm"
                        aria-busy={isVerifying}
                        disabled={busy || isVerifying}
                        onClick={() => onVerify({ provider: hp.key, model: m.id, baseUrl: hp.baseUrl })}
                      >
                        {isVerifying && <Loader2 className="size-4 animate-spin" aria-hidden />}
                        {isVerifying ? "Verifying…" : "Verify tool calling"}
                      </Button>
                    ) : (
                      <Button
                        variant={isActive ? "outline" : "default"}
                        size="sm"
                        disabled={
                          busy || isActive || !hp.keyConfigured || toolsUnsupported || toolsUnverified
                        }
                        onClick={() => onSelect({ provider: hp.key, model: m.id, baseUrl: hp.baseUrl })}
                      >
                        {isActive ? <Check className="size-4" aria-hidden /> : null}
                        {action}
                      </Button>
                    )}
                  </li>
                );
              })}
            </ul>
          ) : null}
        </div>
      );
    })}
  </div>
);

export { HostedModelPicker };

import type { LLMModelView } from "@loomarr/api";
import { formatGiB, formatPercentPoints } from "@loomarr/core";
import { Check, Download, Loader2 } from "lucide-react";
import { Badge, Button } from "@/components/ui";
import { cn } from "@/lib";
import type { ModelPickerProps } from "./model-picker.type";

// The §8.1 model picker. Picking a local model is a real onboarding hurdle — a household
// admin should not have to know which Ollama tag fits their GPU or supports tool-calling
// — so the BE probes the machine, ranks the catalog, and says WHY. This component's job
// is to render that judgement honestly, never to invent its own.
//
// A model that cannot run is shown and disabled rather than hidden: "why isn't X here?"
// is a worse question than seeing X greyed out with the reason next to it.
const FIT_LABEL: Record<string, { text: string; variant: "lock" | "caution" | "onair" }> = {
  fits: { text: "fits", variant: "lock" },
  tight: { text: "tight", variant: "caution" },
  wont_fit: { text: "won't fit", variant: "onair" },
};

const ModelPicker = ({
  catalog,
  active,
  gpuName,
  vramGiB,
  onSelect,
  onPull,
  pulling,
  busy = false,
  className,
}: ModelPickerProps) => (
  <div className={cn("flex flex-col gap-3", className)}>
    {(gpuName || vramGiB) && (
      <p className="text-muted-foreground text-sm">
        Ranked for {gpuName ?? "this machine"}
        {vramGiB ? ` · ${formatGiB(vramGiB)} VRAM` : ""}.
      </p>
    )}

    <ul className="flex flex-col gap-2">
      {catalog.map((model: LLMModelView) => {
        const fit = FIT_LABEL[model.fit] ?? { text: model.fit, variant: "caution" as const };
        const isActive = model.tag === active;
        const isPulling = pulling?.tag === model.tag;
        const progress = isPulling ? pulling?.percent : undefined;
        // Runtime support is a hard blocker in a way VRAM is not: a too-big model is slow,
        // a model the installed Ollama cannot run simply fails.
        const unusable = !model.runtimeOk || model.fit === "wont_fit";

        return (
          <li
            key={model.tag}
            className={cn(
              "flex items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5",
              isActive && "border-signal-tint-30",
            )}
          >
            <div className="min-w-0 flex-1">
              <p className="flex items-center gap-2 font-medium text-sm">
                {model.label}
                <span className="font-mono text-static-400 text-xs">{model.tag}</span>
                {model.recommended && <Badge variant="signal">recommended</Badge>}
                {isActive && <Badge variant="lock">active</Badge>}
              </p>
              <p className="mt-0.5 text-muted-foreground text-sm">{model.why}</p>
              <p className="mt-1 flex items-center gap-2 text-xs">
                <Badge variant={fit.variant}>{fit.text}</Badge>
                <span className="text-static-400">~{formatGiB(model.approxVramGiB)}</span>
                {!model.runtimeOk && <span className="text-onair-300">needs a newer Ollama</span>}
                {isPulling && (
                  <span className="text-tune">
                    downloading{progress !== undefined ? ` ${formatPercentPoints(progress)}` : "…"}
                  </span>
                )}
              </p>
            </div>

            {model.pulled ? (
              <Button
                variant={isActive ? "outline" : "default"}
                size="sm"
                disabled={busy || isActive || unusable}
                onClick={() => onSelect(model.tag)}
              >
                {isActive ? <Check className="size-4" aria-hidden /> : null}
                {isActive ? "In use" : "Use this"}
              </Button>
            ) : (
              // A model must exist locally before it can be selected (§8.1: select 409s
              // on an unpulled tag), so download is the only offer until it is here.
              <Button
                variant="outline"
                size="sm"
                disabled={busy || isPulling || unusable}
                onClick={() => onPull(model.tag)}
              >
                {isPulling ? (
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                ) : (
                  <Download className="size-4" aria-hidden />
                )}
                {isPulling ? "Downloading…" : "Download"}
              </Button>
            )}
          </li>
        );
      })}
    </ul>
  </div>
);

export { ModelPicker };

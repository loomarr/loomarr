import type { LLMModelView } from "@loomarr/api/models/lLMModelView";
import { formatGiB, formatPercentPoints } from "@loomarr/core/format";
import { Action, Badge, type BadgeTone, Stack } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { ModelPickerProps } from "./model-picker.type";

// The §8.1 model picker. Picking a local model is a real onboarding hurdle — a household
// admin should not have to know which Ollama tag fits their GPU or supports tool-calling
// — so the BE probes the machine, ranks the catalog, and says WHY. This component's job
// is to render that judgement honestly, never to invent its own.
//
// A model that cannot run is shown and disabled rather than hidden: "why isn't X here?"
// is a worse question than seeing X greyed out with the reason next to it.
const FIT_LABEL: Record<string, { text: string; tone: BadgeTone }> = {
  fits: { text: "fits", tone: "lock" },
  tight: { text: "tight", tone: "caution" },
  wont_fit: { text: "won't fit", tone: "onair" },
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
  <Stack className={className} gap="control">
    {(gpuName || vramGiB) && (
      <p className="text-muted-foreground text-sm">
        Ranked for {gpuName ?? "this machine"}
        {vramGiB ? ` · ${formatGiB(vramGiB)} VRAM` : ""}.
      </p>
    )}

    <Stack gap="inline" render="ul">
      {catalog.map((model: LLMModelView) => {
        const fit = FIT_LABEL[model.fit] ?? { text: model.fit, tone: "caution" as const };
        const isActive = model.tag === active;
        const isPulling = pulling?.tag === model.tag;
        const progress = isPulling ? pulling?.percent : undefined;
        // Runtime support is a hard blocker in a way VRAM is not: a too-big model is slow,
        // a model the installed Ollama cannot run simply fails.
        const unusable = !model.runtimeOk || model.fit === "wont_fit";

        return (
          <Stack
            align="start"
            className={cn(
              "border-border bg-card rounded-md border px-3 py-2.5",
              // The model in use is neutral, an elevated row like the drawer's tuned one (#1909 Q2).
              isActive && "bg-static-800",
            )}
            direction="row"
            gap="control"
            key={model.tag}
            render="li"
          >
            {/* A model that can't run reads as "here but unavailable" at a glance (§8.1) —
                kept in the list on purpose ("why isn't X here?" is worse than a greyed row).
                Its unavailability is carried by the fit Badge + reason text below, NOT by a
                whole-row `opacity`: opacity composites every descendant (incl. the muted tag)
                below the WCAG AA contrast floor, which the a11y gate rejects (frontend-design
                §7 — express state with color/semantics, never opacity over text). */}
            <div className="min-w-0 flex-1">
              <Stack align="center" className="text-sm font-medium" direction="row" gap="inline" render="p">
                {model.label}
                <span className="text-static-400 font-mono text-xs">{model.tag}</span>
                {model.recommended && (
                  <Badge shape="square" tone="signal">
                    recommended
                  </Badge>
                )}
                {isActive && (
                  <Badge shape="square" tone="lock">
                    active
                  </Badge>
                )}
              </Stack>
              <p className="text-muted-foreground mt-0.5 text-sm">{model.why}</p>
              <Stack align="center" className="mt-1 text-xs" direction="row" gap="inline" render="p">
                <Badge shape="square" tone={fit.tone}>
                  {fit.text}
                </Badge>
                <span className="text-static-400">~{formatGiB(model.approxVramGiB)}</span>
                {!model.runtimeOk && <span className="text-onair-300">needs a newer Ollama</span>}
                {isPulling && (
                  <span className="text-tune">
                    downloading{progress !== undefined ? ` ${formatPercentPoints(progress)}` : "…"}
                  </span>
                )}
              </Stack>
            </div>

            {model.pulled ? (
              <Action
                disabled={busy || isActive || unusable}
                icon={isActive ? "success" : undefined}
                onPress={() => onSelect(model.tag)}
                variant={isActive ? "outline" : undefined}
              >
                {isActive ? "In use" : "Use this"}
              </Action>
            ) : (
              // A model must exist locally before it can be selected (§8.1: select 409s
              // on an unpulled tag), so download is the only offer until it is here.
              <Action
                disabled={busy || isPulling || unusable}
                icon={isPulling ? "loading" : "download"}
                onPress={() => onPull(model.tag)}
                variant="outline"
              >
                {isPulling ? "Downloading…" : "Download"}
              </Action>
            )}
          </Stack>
        );
      })}
    </Stack>
  </Stack>
);

export { ModelPicker };

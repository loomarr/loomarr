import type { HostedModelView } from "@loomarr/api/models/hostedModelView";
import type { HostedProviderView } from "@loomarr/api/models/hostedProviderView";
import { Check } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { HostedModelPickerProps } from "./hosted-model-picker.type";

const GUIDED_ALTERNATIVES = 2;

const rationaleFamily = (model: HostedModelView) => model.why?.split(" — ", 1)[0];

const guidedAlternatives = (models: HostedModelView[], primary?: HostedModelView) => {
  const candidates = models.filter((model) => model.id !== primary?.id);
  const picked: HostedModelView[] = [];
  const seenFamilies = new Set<string>();
  const primaryFamily = primary && rationaleFamily(primary);
  if (primaryFamily) seenFamilies.add(primaryFamily);

  // Prefer distinct ranked families so the shortlist represents real choices rather
  // than two dated or priced variants of the same model line.
  for (const model of candidates) {
    const family = rationaleFamily(model);
    if (!family || seenFamilies.has(family)) continue;
    picked.push(model);
    seenFamilies.add(family);
    if (picked.length === GUIDED_ALTERNATIVES) return picked;
  }
  for (const model of candidates) {
    if (!picked.some((pickedModel) => pickedModel.id === model.id)) picked.push(model);
    if (picked.length === GUIDED_ALTERNATIVES) break;
  }
  return picked;
};

const ModelRow = ({
  model,
  provider,
  activeModel,
  busy,
  primary = false,
  onSelect,
}: {
  model: HostedModelView;
  provider: HostedProviderView;
  activeModel?: string;
  busy: boolean;
  primary?: boolean;
  onSelect: HostedModelPickerProps["onSelect"];
}) => {
  const isActive = provider.active && model.id === activeModel;
  const unusable = !model.tools;
  const action = isActive
    ? "In use"
    : !provider.keyConfigured
      ? "Add key first"
      : unusable
        ? "Can't use"
        : primary
          ? "Use recommended"
          : "Use this";

  return (
    <li
      className={cn(
        "flex items-start gap-3 rounded-md border border-border bg-card px-3 py-2.5",
        primary && "border-signal-tint-30 bg-signal-tint-5",
        isActive && "border-signal-tint-30",
      )}
    >
      <div className="min-w-0 flex-1">
        <p className="flex flex-wrap items-center gap-2 font-medium text-sm">
          {model.label}
          <span className="font-mono text-static-400 text-xs">{model.id}</span>
          {model.recommended && <Badge variant="signal">recommended</Badge>}
          <Badge variant={model.tools ? "lock" : "caution"}>{model.tools ? "Tools" : "Tools required"}</Badge>
          {isActive && <Badge variant="lock">active</Badge>}
        </p>
        {model.why && <p className="mt-0.5 text-muted-foreground text-sm">{model.why}</p>}
        {unusable && !model.why && (
          <p className="mt-0.5 text-muted-foreground text-sm">
            Loomarr requires advertised tool-calling to ground suggestions safely.
          </p>
        )}
      </div>
      <Button
        variant={isActive ? "outline" : "default"}
        size="sm"
        disabled={busy || isActive || !provider.keyConfigured || unusable}
        onClick={() => onSelect({ provider: provider.key, model: model.id, baseUrl: provider.baseUrl })}
      >
        {isActive ? <Check className="size-4" aria-hidden /> : null}
        {action}
      </Button>
    </li>
  );
};

// The §8.1 hosted picker leads non-experts through one safe default and two strong
// alternatives. The provider's complete live catalog remains available as an advanced
// escape hatch, and an active model is never hidden by the collapsed state.
const HostedModelPicker = ({
  providers,
  activeModel,
  onSelect,
  busy = false,
  className,
}: HostedModelPickerProps) => {
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {providers.map((hp: HostedProviderView) => {
        const models = hp.models ?? [];
        const primary = models.find((model) => model.recommended);
        const alternatives = guidedAlternatives(models, primary);
        const guidedIDs = new Set([primary?.id, ...alternatives.map((model) => model.id)]);
        const current = hp.active
          ? models.find((model) => model.id === activeModel && !guidedIDs.has(model.id))
          : undefined;
        const isExpanded = expanded[hp.key] === true;

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
            ) : models.length > 0 && isExpanded ? (
              <>
                <p className="font-medium text-muted-foreground text-xs">All models</p>
                <ul className="flex flex-col gap-2">
                  {models.map((model) => (
                    <ModelRow
                      key={model.id}
                      model={model}
                      provider={hp}
                      activeModel={activeModel}
                      busy={busy}
                      primary={model.id === primary?.id}
                      onSelect={onSelect}
                    />
                  ))}
                </ul>
              </>
            ) : models.length > 0 ? (
              <>
                {primary && (
                  <>
                    <p className="font-medium text-signal text-xs">Safe default</p>
                    <ul>
                      <ModelRow
                        model={primary}
                        provider={hp}
                        activeModel={activeModel}
                        busy={busy}
                        primary
                        onSelect={onSelect}
                      />
                    </ul>
                  </>
                )}
                {alternatives.length > 0 && (
                  <>
                    <p className="mt-1 font-medium text-muted-foreground text-xs">
                      {primary ? "Strong alternatives" : "Available models"}
                    </p>
                    <ul className="flex flex-col gap-2">
                      {alternatives.map((model) => (
                        <ModelRow
                          key={model.id}
                          model={model}
                          provider={hp}
                          activeModel={activeModel}
                          busy={busy}
                          onSelect={onSelect}
                        />
                      ))}
                    </ul>
                  </>
                )}
                {current && (
                  <>
                    <p className="mt-1 font-medium text-muted-foreground text-xs">Currently active</p>
                    <ul>
                      <ModelRow
                        model={current}
                        provider={hp}
                        activeModel={activeModel}
                        busy={busy}
                        onSelect={onSelect}
                      />
                    </ul>
                  </>
                )}
              </>
            ) : null}

            {models.length > (primary ? GUIDED_ALTERNATIVES + 1 : GUIDED_ALTERNATIVES) && (
              <Button
                variant="ghost"
                size="sm"
                className="self-start text-muted-foreground text-xs"
                onClick={() => setExpanded((value) => ({ ...value, [hp.key]: !isExpanded }))}
              >
                {isExpanded ? "Show guided choices" : `Show all ${models.length} models`}
              </Button>
            )}
          </div>
        );
      })}
    </div>
  );
};

export { HostedModelPicker };

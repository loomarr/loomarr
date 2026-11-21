import type { HostedModelView } from "@loomarr/api/models/hostedModelView";
import type { HostedProviderView } from "@loomarr/api/models/hostedProviderView";
import { Action, Badge, Card, Stack, Text, TextLink } from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { useState } from "react";
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
    // The model in use is neutral, an elevated row like the drawer's tuned one (#1909 Q2).
    <Card
      alignItems="flex-start"
      borderRadius={6}
      flexDirection="row"
      gap={12}
      level={isActive ? "elevated" : "raised"}
      paddingHorizontal={12}
      paddingVertical={10}
      render="li"
      tone={primary && !isActive ? "signal" : undefined}
    >
      <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
        <Stack align="center" direction="row" gap="inline" render="p" wrap>
          <Text as="span" textRole="cardLabel">
            {model.label}
          </Text>
          <Text as="span" textRole="mono" tone="secondary">
            {model.id}
          </Text>
          {model.recommended && (
            <Badge shape="square" tone="signal">
              recommended
            </Badge>
          )}
          <Badge shape="square" tone={model.tools ? "lock" : "caution"}>
            {model.tools ? "Tools" : "Tools required"}
          </Badge>
          {isActive && (
            <Badge shape="square" tone="lock">
              active
            </Badge>
          )}
        </Stack>
        {model.why && (
          <Stack marginTop={2}>
            <Text as="p" textRole="compact" tone="secondary">
              {model.why}
            </Text>
          </Stack>
        )}
        {unusable && !model.why && (
          <Stack marginTop={2}>
            <Text as="p" textRole="compact" tone="secondary">
              Loomarr requires advertised tool-calling to ground suggestions safely.
            </Text>
          </Stack>
        )}
      </Stack>
      <Action
        disabled={busy || isActive || !provider.keyConfigured || unusable}
        icon={isActive ? "success" : undefined}
        onPress={() => onSelect({ provider: provider.key, model: model.id, baseUrl: provider.baseUrl })}
        variant={isActive ? "outline" : undefined}
      >
        {action}
      </Action>
    </Card>
  );
};

// The §8.1 hosted picker leads non-experts through one safe default and two strong
// alternatives. A bounded typeahead reaches the provider's complete live catalog without
// rendering hundreds of choices, and an active model is never hidden by the guided state.
const HostedModelPicker = ({
  providers,
  activeModel,
  onSelect,
  busy = false,
  className,
}: HostedModelPickerProps) => {
  const [searching, setSearching] = useState<Record<string, boolean>>({});
  const [queries, setQueries] = useState<Record<string, string>>({});

  return (
    <Stack className={className} gap="4">
      {providers.map((hp: HostedProviderView) => {
        const models = hp.models ?? [];
        const primary = models.find((model) => model.recommended);
        const alternatives = guidedAlternatives(models, primary);
        const guidedIDs = new Set([primary?.id, ...alternatives.map((model) => model.id)]);
        const current = hp.active
          ? models.find((model) => model.id === activeModel && !guidedIDs.has(model.id))
          : undefined;
        const isSearching = searching[hp.key] === true;
        const query = queries[hp.key] ?? "";
        const normalizedQuery = query.trim().toLowerCase();
        const matchingModels = normalizedQuery
          ? models
              .filter(
                (model) =>
                  model.label.toLowerCase().includes(normalizedQuery) ||
                  model.id.toLowerCase().includes(normalizedQuery),
              )
              .slice(0, 20)
          : [];

        return (
          <Stack gap="inline" key={hp.key}>
            <Stack align="baseline" direction="row" gap="inline" justify="between">
              <Text as="p" textRole="cardLabel">
                {hp.label}
                {hp.active && (
                  <Text as="span" className="ml-2" textRole="hint" tone="secondary">
                    (in use)
                  </Text>
                )}
              </Text>
              {hp.modelsLive ? (
                <Text as="span" textRole="hint" tone="secondary">
                  live models
                </Text>
              ) : (
                <Text as="span" textRole="hint" tone="secondary">
                  suggested
                </Text>
              )}
            </Stack>

            {!hp.keyConfigured && (
              <Text as="p" textRole="compact" tone="secondary">
                {hp.key === "openrouter"
                  ? "Add your OpenRouter key above and save to use the suggested model and load live options."
                  : "Add this provider's URL + key above and save to load its models."}
                {hp.keysUrl && (
                  <>
                    {" "}
                    <TextLink external href={hp.keysUrl}>
                      Get a key
                    </TextLink>
                  </>
                )}
              </Text>
            )}

            {hp.keyConfigured && models.length === 0 ? (
              <Text as="p" textRole="compact" tone="secondary">
                No models returned. Check the key and base URL above, then Test again.
              </Text>
            ) : models.length > 0 && isSearching ? (
              <>
                <SearchField
                  autoFocus
                  label="Search models"
                  labelHidden
                  onChangeText={(nextQuery) => setQueries((value) => ({ ...value, [hp.key]: nextQuery }))}
                  placeholder={`Search ${models.length} models by name or ID`}
                  value={query}
                />
                {!normalizedQuery ? (
                  <Text as="p" textRole="compact" tone="secondary">
                    Start typing to find a model.
                  </Text>
                ) : matchingModels.length === 0 ? (
                  <div role="status">
                    <Text as="p" textRole="compact" tone="secondary">
                      No models match “{query.trim()}”.
                    </Text>
                  </div>
                ) : (
                  <Stack gap="inline" render="ul">
                    {matchingModels.map((model) => (
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
                  </Stack>
                )}
                {matchingModels.length === 20 && (
                  <Text as="p" textRole="hint" tone="secondary">
                    Showing the first 20 matches. Keep typing to narrow them.
                  </Text>
                )}
              </>
            ) : models.length > 0 ? (
              <>
                {primary && (
                  <>
                    <Text as="p" textRole="label" tone="signal">
                      Safe default
                    </Text>
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
                    <Stack marginTop={4}>
                      <Text as="p" textRole="label">
                        {primary ? "Strong alternatives" : "Available models"}
                      </Text>
                    </Stack>
                    <Stack gap="inline" render="ul">
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
                    </Stack>
                  </>
                )}
                {current && (
                  <>
                    <Stack marginTop={4}>
                      <Text as="p" textRole="label">
                        Currently active
                      </Text>
                    </Stack>
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
              <Action
                onPress={() => setSearching((value) => ({ ...value, [hp.key]: !isSearching }))}
                style={{ alignSelf: "flex-start" }}
                variant="ghost"
              >
                {isSearching ? "Back to recommendations" : "Find another model"}
              </Action>
            )}
          </Stack>
        );
      })}
    </Stack>
  );
};

export { HostedModelPicker };

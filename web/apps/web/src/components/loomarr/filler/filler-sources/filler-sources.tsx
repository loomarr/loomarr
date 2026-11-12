import { formatRelative } from "@loomarr/core/format";
import { Action, Icon, icons, Stack, type StatusTone, Surface, Toggle } from "@loomarr/design-system";
import { CardAction, CardStatus, ListGroup, RowCard, SearchField } from "@loomarr/ui";
import { type ReactNode, useState } from "react";
import { cn } from "@/lib/utils";
import type { FillerSourcesProps } from "./filler-sources.type";

const dormant = (source: { effectiveEnabled: boolean }) => !source.effectiveEnabled;
const FILTER_THRESHOLD = 10;
const INITIAL_HEALTHY = 5;
type Source = FillerSourcesProps["sources"][number];
const needsAttention = (source: Source) =>
  source.readiness === "needs_location" ||
  source.readiness === "not_configured" ||
  source.readiness === "out_of_area" ||
  source.readiness === "unavailable";

const sourceSummary = (sources: Source[]) => {
  if (sources.length === 0) return "Nothing added";
  const attention = sources.filter(needsAttention).length;
  const count = `${sources.length} ${sources.length === 1 ? "source" : "sources"}`;
  if (attention === 0) return count;
  return `${count} · ${attention} ${attention === 1 ? "needs" : "need"} attention`;
};

interface SourceListProps {
  sources: Source[];
  sectionLabel: string;
  renderRow: (source: Source) => ReactNode;
  className?: string;
}

// Long lists keep problems visible without letting healthy rows take over the page. Filtering is
// deliberately local to registered rows; the provider finder above this list remains independent.
const SourceList = ({ sources, sectionLabel, renderRow, className }: SourceListProps) => {
  const [expanded, setExpanded] = useState(false);
  const [filter, setFilter] = useState("");
  const scalable = sources.length >= FILTER_THRESHOLD;
  const normalizedFilter = filter.trim().toLocaleLowerCase();
  const filtered = normalizedFilter
    ? sources.filter((source) =>
        [source.target, source.detail, source.uri]
          .filter(Boolean)
          .some((value) => value?.toLocaleLowerCase().includes(normalizedFilter)),
      )
    : sources;
  const attention = filtered.filter(needsAttention);
  const paused = filtered.filter(
    (source) =>
      !needsAttention(source) && (source.readiness === "off" || source.readiness === "provider_off"),
  );
  const healthy = filtered.filter((source) => !needsAttention(source) && !paused.includes(source));
  const ordered = scalable ? [...attention, ...paused, ...healthy] : filtered;
  const visible =
    scalable && !normalizedFilter && !expanded
      ? [...attention, ...paused, ...healthy.slice(0, INITIAL_HEALTHY)]
      : ordered;
  const remaining =
    scalable && !normalizedFilter && !expanded ? Math.max(healthy.length - INITIAL_HEALTHY, 0) : 0;

  return (
    <div className={className}>
      {scalable && (
        <Surface backgroundColor="transparent" borderWidth={0} maxWidth={384}>
          <SearchField
            label={`Filter ${sectionLabel} sources`}
            labelHidden
            onChangeText={setFilter}
            placeholder="Filter your sources…"
            value={filter}
          />
        </Surface>
      )}

      {visible.length > 0 ? (
        <div className={cn(scalable && "mt-2")}>
          <ListGroup
            aria-label={sectionLabel === "Your files" ? "Your file sources" : `Sources under ${sectionLabel}`}
          >
            {visible.map(renderRow)}
          </ListGroup>
        </div>
      ) : (
        <p className="text-muted-foreground mt-3 text-sm">No sources match your filter.</p>
      )}

      {remaining > 0 && (
        <Stack className="mt-2" direction="row">
          <Action
            accessibilityLabel={`Show ${remaining} more under ${sectionLabel}`}
            onPress={() => setExpanded(true)}
            variant="ghost"
          >
            {`Show ${remaining} more`}
          </Action>
        </Stack>
      )}
      {scalable && expanded && !normalizedFilter && (
        <Stack className="mt-2" direction="row">
          <Action
            accessibilityLabel={`Show fewer under ${sectionLabel}`}
            onPress={() => setExpanded(false)}
            variant="ghost"
          >
            Show fewer
          </Action>
        </Stack>
      )}
    </div>
  );
};

// A calm read of source policy. Provider headings already say what each remote is, so rows do not
// repeat that information with coloured type chips or nested cards.
const FillerSources = ({
  sources,
  onSelect,
  selectedId,
  onToggleEnabled,
  toggling,
  onToggleProvider,
  togglingProvider,
  renderProviderSetup,
  renderLocalSetup,
  descriptions = [],
  error,
  now,
  className,
}: FillerSourcesProps) => {
  const childrenByProvider = new Map<string, typeof sources>();
  for (const source of sources) {
    if (!source.parentId) continue;
    childrenByProvider.set(source.parentId, [...(childrenByProvider.get(source.parentId) ?? []), source]);
  }
  const providers = sources.filter((source) => source.group);
  // A provider's row carries its operator-facing name, which its paused children quote.
  const providerRowsByID = new Map(providers.map((provider) => [provider.id, provider]));
  const localSources = sources.filter((source) => !source.group && !source.parentId);
  const realSources = sources.filter((source) => !source.group);
  const needsLocation = realSources.some((source) => source.actions.includes("set_location"));

  const row = (source: (typeof sources)[number]) => {
    const name = source.id === "folder" ? "Drop folder" : source.target;
    const counts = [
      source.incoming > 0
        ? `${source.count} ready`
        : `${source.count} ${source.count === 1 ? "clip" : "clips"}`,
      ...(source.incoming > 0 ? [`${source.incoming} being checked`] : []),
      ...(source.lastCheckedAt ? [`checked ${formatRelative(source.lastCheckedAt, now)}`] : []),
    ].join(" · ");
    // The row's one status: what needs the operator, else off, else ready.
    const health: { label: string; tone: StatusTone } =
      source.readiness === "not_configured"
        ? { label: "Add details", tone: "warn" }
        : source.readiness === "needs_location" && !needsLocation
          ? { label: "Location needed", tone: "warn" }
          : source.readiness === "out_of_area"
            ? { label: "Not used here", tone: "warn" }
            : source.readiness === "unavailable"
              ? { label: "Needs attention", tone: "warn" }
              : dormant(source)
                ? { label: "Off", tone: "off" }
                : { label: "Ready", tone: "ok" };
    const detail =
      source.readiness === "provider_off"
        ? `Paused with ${providerRowsByID.get(source.parentId ?? "")?.target ?? source.kind}. Existing clips stay here.`
        : dormant(source)
          ? "Paused. Existing clips stay in your library."
          : source.id === "folder"
            ? source.configured
              ? source.target
              : "Choose a folder for files you add yourself."
            : source.detail;
    const meta = [
      <CardStatus key="status" label={health.label} tone={health.tone} />,
      ...(dormant(source) ? [] : [counts]),
      ...(detail ? [detail] : []),
      ...(source.readiness === "ready" && source.locationSource === "source" && source.effectiveCountry
        ? [source.effectiveCountry]
        : []),
      ...(source.onForPlace && source.enabled ? ["For your area"] : []),
      ...(source.license ? [source.license] : []),
    ];
    const lead = source.parentId ? (
      <Icon decorative glyph={icons.archive} size="default" tone="muted" />
    ) : (
      <Icon decorative glyph={icons.folder} size="default" tone="muted" />
    );
    return (
      <div
        key={source.id}
        className={cn("flex min-w-0 items-center gap-2 pl-2", selectedId === source.id && "bg-muted/25")}
      >
        {onToggleEnabled && source.switchable && (
          <Toggle
            kind="switch"
            id={`source-use-${source.id}`}
            label={`Use ${source.target}`}
            labelHidden
            checked={source.enabled}
            disabled={
              toggling != null ||
              !source.providerEnabled ||
              (source.parentId != null && togglingProvider === source.kind)
            }
            onCheckedChange={(checked) => onToggleEnabled(source.id, checked)}
          />
        )}
        <div className="min-w-0 flex-1">
          <RowCard
            action={
              onSelect ? (
                <CardAction
                  accessibilityLabel={`Manage ${name}`}
                  attention={health.tone === "warn"}
                  onPress={() => onSelect(source)}
                >
                  Manage
                </CardAction>
              ) : undefined
            }
            lead={lead}
            meta={meta}
            title={name}
          />
        </div>
      </div>
    );
  };

  return (
    <Stack className={className} gap="section">
      <div className="flex flex-wrap items-start gap-4">
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold">Where filler comes from</h2>
          <p className="text-muted-foreground mt-1 max-w-xl text-sm">
            Add sources once and Loomarr checks them for you. Turning one off does not remove clips already
            added.
          </p>
        </div>
        <span className="text-muted-foreground shrink-0 text-xs whitespace-nowrap">
          {`${realSources.filter((source) => source.ready).length} of ${realSources.length} ready`}
        </span>
      </div>

      {needsLocation && (
        <Stack
          align="center"
          className="border-caution/30 bg-caution/5 rounded-lg border px-4 py-3"
          direction="row"
          gap="control"
          justify="between"
          wrap
        >
          <div>
            <p className="text-sm font-medium">Add your location</p>
            <p className="text-muted-foreground text-sm">
              Loomarr uses it to choose sources that fit your area.
            </p>
          </div>
          <Action accessibilityRole="link" render={<a href="/settings/access" />} variant="outline">
            Set location
          </Action>
        </Stack>
      )}

      {error && <p className="text-onair-300 text-sm">{error}</p>}

      {(localSources.length > 0 || renderLocalSetup) && (
        <section aria-labelledby="local-filler-sources" className="border-border/60 border-t pt-5">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <h3 id="local-filler-sources" className="font-medium">
                Your files
              </h3>
              <p className="text-muted-foreground mt-1 text-sm">
                Folders and media-server libraries you already manage.
              </p>
            </div>
            <span className="text-muted-foreground shrink-0 text-xs">{sourceSummary(localSources)}</span>
          </div>

          {renderLocalSetup && <div className="mt-4">{renderLocalSetup}</div>}
          {localSources.length > 0 && (
            <SourceList
              className={renderLocalSetup ? "mt-4" : "mt-2"}
              sources={localSources}
              sectionLabel="Your files"
              renderRow={row}
            />
          )}
        </section>
      )}

      {providers.map((provider) => {
        const children = childrenByProvider.get(provider.id) ?? [];
        const providerBodyOpen = provider.enabled && togglingProvider !== provider.kind;
        return (
          <section
            key={provider.id}
            aria-labelledby={`provider-${provider.kind}`}
            className="border-border/60 border-t pt-5"
          >
            <div className="flex flex-wrap items-start justify-between gap-4 sm:flex-nowrap">
              <div className="min-w-0 basis-full sm:flex-1 sm:basis-0">
                <h3 id={`provider-${provider.kind}`} className="font-medium">
                  {provider.target}
                </h3>
                <p className="text-muted-foreground mt-1 text-sm">
                  {!provider.enabled
                    ? `${provider.target} is paused. Sources and clips stay saved.`
                    : descriptions.find((description) => description.id === provider.kind)?.blurb}
                </p>
              </div>
              <div className="ml-auto flex shrink-0 items-center gap-3">
                <span className="text-muted-foreground text-xs">{sourceSummary(children)}</span>
                {onToggleProvider && (
                  <Toggle
                    kind="switch"
                    id={`provider-use-${provider.kind}`}
                    label={`Use ${provider.target}`}
                    labelHidden
                    checked={provider.enabled}
                    disabled={togglingProvider != null}
                    onCheckedChange={(checked) => onToggleProvider(provider.kind, checked)}
                  />
                )}
              </div>
            </div>

            {/* Folded, not unmounted: a paused provider's children keep their saved switches. */}
            <div hidden={!providerBodyOpen}>
              {renderProviderSetup?.(provider)}

              {children.length > 0 ? (
                <SourceList
                  className="mt-3"
                  sources={children}
                  sectionLabel={provider.target}
                  renderRow={row}
                />
              ) : (
                <p className="text-muted-foreground mt-3 text-xs">Nothing added yet.</p>
              )}
            </div>
          </section>
        );
      })}
    </Stack>
  );
};

export { FillerSources };

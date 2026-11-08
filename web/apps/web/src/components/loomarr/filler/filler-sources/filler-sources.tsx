import { formatRelative } from "@loomarr/core/format";
import { Action, Badge, Surface, Toggle } from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { ChevronRight } from "lucide-react";
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
        <ul
          className={cn("flex flex-col gap-1", scalable && "mt-2")}
          aria-label={sectionLabel === "Your files" ? "Your file sources" : `Sources under ${sectionLabel}`}
        >
          {visible.map(renderRow)}
        </ul>
      ) : (
        <p className="text-muted-foreground mt-3 text-sm">No sources match your filter.</p>
      )}

      {remaining > 0 && (
        <div className="mt-2 flex">
          <Action
            accessibilityLabel={`Show ${remaining} more under ${sectionLabel}`}
            onPress={() => setExpanded(true)}
            variant="ghost"
          >
            {`Show ${remaining} more`}
          </Action>
        </div>
      )}
      {scalable && expanded && !normalizedFilter && (
        <div className="mt-2 flex">
          <Action
            accessibilityLabel={`Show fewer under ${sectionLabel}`}
            onPress={() => setExpanded(false)}
            variant="ghost"
          >
            Show fewer
          </Action>
        </div>
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
    const status = dormant(source)
      ? "Off"
      : [
          source.incoming > 0
            ? `${source.count} ready`
            : `${source.count} ${source.count === 1 ? "clip" : "clips"}`,
          ...(source.incoming > 0 ? [`${source.incoming} being checked`] : []),
          ...(source.lastCheckedAt ? [`checked ${formatRelative(source.lastCheckedAt, now)}`] : []),
        ].join(" · ");
    const details = (
      <>
        <div className="min-w-0 basis-full text-left sm:flex-1 sm:basis-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="truncate text-sm font-medium">
              {source.id === "folder" ? "Drop folder" : source.target}
            </span>
            {source.readiness === "not_configured" && (
              <Badge shape="square" tone="caution">
                Add details
              </Badge>
            )}
            {source.readiness === "needs_location" && !needsLocation && (
              <Badge shape="square" tone="caution">
                Location needed
              </Badge>
            )}
            {source.readiness === "out_of_area" && (
              <Badge shape="square" tone="caution">
                Not used here
              </Badge>
            )}
            {source.readiness === "unavailable" && (
              <Badge shape="square" tone="caution">
                Needs attention
              </Badge>
            )}
            {source.readiness === "ready" &&
              source.locationSource === "source" &&
              source.effectiveCountry && (
                <Badge shape="square" tone="neutral">
                  {source.effectiveCountry}
                </Badge>
              )}
            {source.onForPlace && source.enabled && (
              <Badge shape="square" tone="neutral">
                For your area
              </Badge>
            )}
          </div>
          <p className="text-muted-foreground mt-0.5 truncate text-xs">
            {source.readiness === "provider_off"
              ? `Paused with ${providerRowsByID.get(source.parentId ?? "")?.target ?? source.kind}. Existing clips stay here.`
              : dormant(source)
                ? "Paused. Existing clips stay in your library."
                : source.id === "folder"
                  ? source.configured
                    ? source.target
                    : "Choose a folder for files you add yourself."
                  : source.detail}
          </p>
        </div>
        <span className="text-muted-foreground shrink-0 text-xs whitespace-nowrap">{status}</span>
        {source.license && <span className="text-muted-foreground shrink-0 text-xs">{source.license}</span>}
        {onSelect && <ChevronRight className="text-muted-foreground size-4 shrink-0" aria-hidden />}
      </>
    );
    return (
      <li
        key={source.id}
        className={cn(
          "py-1",
          source.parentId
            ? "border-border/40 border-b px-1 last:border-b-0"
            : "border-border/50 border-b last:border-b-0",
          selectedId === source.id && "bg-muted/25",
        )}
      >
        <div className="flex min-w-0 items-center gap-2">
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
          {onSelect ? (
            <button
              type="button"
              className="hover:bg-muted/50 focus-visible:ring-ring relative flex min-w-0 flex-1 cursor-pointer flex-wrap items-center gap-3 rounded-md px-2 py-2 focus-visible:ring-2 focus-visible:outline-none sm:flex-nowrap"
              onClick={() => onSelect(source)}
            >
              {/* The name comes from the content, so it holds every visible word (WCAG 2.5.3);
                  the verb is the one thing the row shows only to assistive tech. */}
              <span className="sr-only">Manage</span> {details}
            </button>
          ) : (
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-3 px-2 py-2 sm:flex-nowrap">
              {details}
            </div>
          )}
        </div>
      </li>
    );
  };

  return (
    <div className={cn("flex flex-col gap-6", className)}>
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
        <div className="border-caution/30 bg-caution/5 flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3">
          <div>
            <p className="text-sm font-medium">Add your location</p>
            <p className="text-muted-foreground text-sm">
              Loomarr uses it to choose sources that fit your area.
            </p>
          </div>
          <Action accessibilityRole="link" render={<a href="/settings/access" />} variant="outline">
            Set location
          </Action>
        </div>
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
    </div>
  );
};

export { FillerSources };

import { formatRelative } from "@loomarr/core/format";
import { ChevronRight } from "lucide-react";
import { type ReactNode, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Disclosure } from "@/components/ui/disclosure";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
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
        <Input
          type="search"
          className="max-w-sm"
          value={filter}
          placeholder="Filter your sources…"
          aria-label={`Filter ${sectionLabel} sources`}
          onChange={(event) => setFilter(event.target.value)}
        />
      )}

      {visible.length > 0 ? (
        <ul
          className={cn("flex flex-col gap-1", scalable && "mt-2")}
          aria-label={sectionLabel === "Your files" ? "Your file sources" : `Sources under ${sectionLabel}`}
        >
          {visible.map(renderRow)}
        </ul>
      ) : (
        <p className="mt-3 text-muted-foreground text-sm">No sources match your filter.</p>
      )}

      {remaining > 0 && (
        <Button
          type="button"
          variant="ghost"
          className="mt-2"
          aria-label={`Show ${remaining} more under ${sectionLabel}`}
          onClick={() => setExpanded(true)}
        >
          Show {remaining} more
        </Button>
      )}
      {scalable && expanded && !normalizedFilter && (
        <Button
          type="button"
          variant="ghost"
          className="mt-2"
          aria-label={`Show fewer under ${sectionLabel}`}
          onClick={() => setExpanded(false)}
        >
          Show fewer
        </Button>
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
  error,
  className,
}: FillerSourcesProps) => {
  const childrenByProvider = new Map<string, typeof sources>();
  for (const source of sources) {
    if (!source.parentId) continue;
    childrenByProvider.set(source.parentId, [...(childrenByProvider.get(source.parentId) ?? []), source]);
  }
  const providers = sources.filter((source) => source.group);
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
          ...(source.lastCheckedAt ? [`checked ${formatRelative(source.lastCheckedAt)}`] : []),
        ].join(" · ");
    const details = (
      <>
        <div className="min-w-0 basis-full text-left sm:flex-1 sm:basis-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <span className="truncate font-medium text-sm">
              {source.id === "folder" ? "Drop folder" : source.target}
            </span>
            {source.readiness === "not_configured" && <Badge variant="caution">Add details</Badge>}
            {source.readiness === "needs_location" && !needsLocation && (
              <Badge variant="caution">Location needed</Badge>
            )}
            {source.readiness === "out_of_area" && <Badge variant="caution">Not used here</Badge>}
            {source.readiness === "unavailable" && <Badge variant="caution">Needs attention</Badge>}
            {source.readiness === "ready" &&
              source.locationSource === "source" &&
              source.effectiveCountry && (
                <Badge variant="neutral">
                  {source.effectiveMarket
                    ? `${source.effectiveCountry} · ${source.effectiveMarket}`
                    : source.effectiveCountry}
                </Badge>
              )}
          </div>
          <p className="mt-0.5 truncate text-muted-foreground text-xs">
            {source.readiness === "provider_off"
              ? `Paused with ${source.kind === "archive" ? "Archive.org" : "YouTube"}. Existing clips stay here.`
              : dormant(source)
                ? "Paused. Existing clips stay in your library."
                : source.id === "folder"
                  ? source.configured
                    ? source.target
                    : "Choose a folder for files you add yourself."
                  : source.detail}
          </p>
        </div>
        <span className="shrink-0 whitespace-nowrap text-muted-foreground text-xs">{status}</span>
        {source.license && <span className="shrink-0 text-muted-foreground text-xs">{source.license}</span>}
        {onSelect && <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
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
            <Switch
              id={`source-use-${source.id}`}
              checked={source.enabled}
              disabled={
                toggling != null ||
                !source.providerEnabled ||
                (source.parentId != null && togglingProvider === source.kind)
              }
              onChange={() => onToggleEnabled(source.id, !source.enabled)}
              aria-label={`Use ${source.target}`}
              className="mt-0.5 shrink-0"
            />
          )}
          {onSelect ? (
            <button
              type="button"
              className="flex min-w-0 flex-1 cursor-pointer flex-wrap items-center gap-3 rounded-md px-2 py-2 hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:flex-nowrap"
              aria-label={`Manage ${source.id === "folder" ? "Drop folder" : source.target}`}
              onClick={() => onSelect(source)}
            >
              {details}
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
          <h2 className="font-semibold text-sm">Where filler comes from</h2>
          <p className="mt-1 max-w-xl text-muted-foreground text-sm">
            Add sources once and Loomarr checks them for you. Turning one off does not remove clips already
            added.
          </p>
        </div>
        <span className="shrink-0 whitespace-nowrap text-muted-foreground text-xs">
          {`${realSources.filter((source) => source.ready).length} of ${realSources.length} ready`}
        </span>
      </div>

      {needsLocation && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-caution/30 bg-caution/5 px-4 py-3">
          <div>
            <p className="font-medium text-sm">Add your location</p>
            <p className="text-muted-foreground text-sm">
              Loomarr uses it to choose sources that fit your area.
            </p>
          </div>
          <a className={buttonVariants({ variant: "outline" })} href="/settings/general">
            Set location
          </a>
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
              <p className="mt-1 text-muted-foreground text-sm">
                Folders and media-server libraries you already manage.
              </p>
            </div>
            <span className="shrink-0 text-muted-foreground text-xs">{sourceSummary(localSources)}</span>
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
        const providerKind =
          provider.kind === "archive" || provider.kind === "youtube" ? provider.kind : undefined;
        const providerBodyOpen = provider.enabled && togglingProvider !== providerKind;
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
                <p className="mt-1 text-muted-foreground text-sm">
                  {!provider.enabled
                    ? `${provider.target} is paused. Sources and clips stay saved.`
                    : provider.kind === "archive"
                      ? "Public collections of commercials, bumpers, and other broadcast material."
                      : "Channels and playlists you choose from YouTube."}
                </p>
              </div>
              <div className="ml-auto flex shrink-0 items-center gap-3">
                <span className="text-muted-foreground text-xs">{sourceSummary(children)}</span>
                {onToggleProvider && providerKind && (
                  <Switch
                    id={`provider-use-${provider.kind}`}
                    checked={provider.enabled}
                    disabled={togglingProvider != null}
                    onChange={() => onToggleProvider(providerKind, !provider.enabled)}
                    aria-label={`Use ${provider.target}`}
                  />
                )}
              </div>
            </div>

            <Disclosure open={providerBodyOpen}>
              <Disclosure.Panel>
                {renderProviderSetup?.(provider)}

                {children.length > 0 ? (
                  <SourceList
                    className="mt-3"
                    sources={children}
                    sectionLabel={provider.target}
                    renderRow={row}
                  />
                ) : (
                  <p className="mt-3 text-muted-foreground text-xs">Nothing added yet.</p>
                )}
              </Disclosure.Panel>
            </Disclosure>
          </section>
        );
      })}
    </div>
  );
};

export { FillerSources };

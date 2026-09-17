import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative, pluralize } from "@loomarr/core/format";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { PoolHealth } from "@/components/loomarr/filler/pool-health";
import { WatchPill } from "@/components/loomarr/filler/watch-pill";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { NavTabs } from "@/components/ui/nav-tabs";
import { useDocumentTitle } from "@/lib/use-document-title";
import { FillerCatalog } from "../filler-catalog";
import { FillerManage } from "../filler-manage";
import { FillerOverview } from "../filler-overview";
import type { FillerSearch } from "../filler-search";
import { FillerSettings, FillerSettingsIndex } from "../filler-settings";
import { Incoming } from "../incoming";
import { SourcesTab } from "../sources-tab";
import { TaxonomyTab } from "../taxonomy-tab";
import type { FillerPageProps } from "./filler-page.type";

// FillerPage is the route-level composition root. It owns only state shared across destinations:
// navigation counts, installation readiness, the watch/health summary. Destination-specific behavior stays in its own module.
const FillerPage = ({ tab, settingsSection }: FillerPageProps) => {
  useDocumentTitle("Filler");
  const navigate = useNavigate();
  const { isAdmin } = useAuth();

  // Preserve Library's deep-linkable state when an operator leaves and returns. The shell only
  // carries the opaque route state; FillerCatalog owns its interpretation and mutations.
  const search = useSearch({ strict: false }) as Partial<FillerSearch>;
  const catalogSearch = {
    ...(search.q ? { q: search.q } : {}),
    ...(search.kind ? { kind: search.kind } : {}),
    ...(search.audience ? { audience: search.audience } : {}),
    ...(search.taxon ? { taxon: search.taxon } : {}),
    ...(search.unclassified ? { unclassified: true } : {}),
    ...(search.withoutAxis ? { withoutAxis: search.withoutAxis } : {}),
    ...(search.untagged ? { untagged: true } : {}),
    ...(search.view && search.view !== "grid" ? { view: search.view } : {}),
    ...(search.page && search.page > 1 ? { page: search.page } : {}),
    ...(search.parent ? { parent: search.parent } : {}),
  };

  const settings = settingsApi.useSettingsList();
  const fillerConfigured = Boolean(unwrap(settings.data, (body) => body.features)?.filler);
  const watch = unwrap(fillerApi.useFillerWatch().data, (body) => body);
  const poolQuery = fillerApi.useFillerPool({ query: { enabled: tab === "library" } });
  const pool = unwrap(poolQuery.data, (body) => body);

  if (!fillerConfigured && tab !== "settings") {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <PageHeader title="Filler" description={<FillerDescription />} />
        <div className="min-h-0 flex-1 overflow-auto p-6">
          <EmptyState
            title="No filler folder configured"
            description="Choose a folder for commercials, bumpers, and station IDs. Loomarr indexes it for scheduling; point Tunarr at the same folder so it can play the clips."
            {...(isAdmin
              ? {
                  action: {
                    label: "Choose clip folders",
                    onClick: () =>
                      navigate({ to: "/filler/settings/$section", params: { section: "folders" } }),
                  },
                }
              : {})}
          />
        </div>
      </div>
    );
  }

  const statusLine = watch
    ? [
        `${watch.sourcesReady} of ${watch.sourcesTotal} sources ready`,
        pluralize(watch.clips, "clip"),
        ...(watch.held > 0 ? [`${watch.held} waiting`] : []),
        ...(watch.lastScanAt ? [`last scan ${formatRelative(watch.lastScanAt)}`] : []),
      ].join(" · ")
    : "";

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="Filler"
        description={<FillerDescription />}
        actions={watch && <WatchPill status={statusLine} health={watch.health} />}
      />
      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-auto p-6">
        <NavTabs
          label="Filler sections"
          linkComponent={Link}
          tabs={[
            { id: "overview", label: "Overview", to: "/filler" },
            ...(isAdmin
              ? [
                  { id: "sources", label: "Sources", to: "/filler/sources" },
                  { id: "incoming", label: "Incoming", to: "/filler/incoming" },
                ]
              : []),
            {
              id: "library",
              label: "Library",
              to: "/filler/library",
              search: catalogSearch,
              count: watch?.clips,
            },
            { id: "manage", label: "Manage", to: "/filler/manage" },
          ]}
          activeId={tab === "taxonomy" || tab === "settings" ? "manage" : tab}
        />

        {pool && tab === "library" ? <PoolHealth pool={pool} /> : null}

        {watch?.autoFetch?.stoppedBy === "catalog" && tab === "library" ? (
          <Card className="flex flex-wrap items-center gap-3 border-caution/40 bg-caution/5 p-4">
            <div className="min-w-0 flex-1">
              <p className="font-medium text-sm">Automatic fetching is paused</p>
              <p className="mt-0.5 text-muted-foreground text-sm">
                {watch.autoFetch.catalogClips.toLocaleString()} of{" "}
                {(watch.autoFetch.maxCatalog ?? 0).toLocaleString()} catalog clips are in use. You can still
                add clips yourself. Remove clips you no longer want or raise this limit to restart automatic
                downloads.
              </p>
            </div>
            {isAdmin ? (
              <Button
                variant="outline"
                size="sm"
                render={<Link to="/filler/settings/$section" params={{ section: "storage" }} />}
              >
                Review limits
              </Button>
            ) : null}
          </Card>
        ) : null}

        {tab === "overview" ? (
          <FillerOverview />
        ) : tab === "incoming" && isAdmin ? (
          <Incoming />
        ) : tab === "manage" ? (
          <FillerManage />
        ) : tab === "sources" ? (
          <SourcesTab />
        ) : tab === "taxonomy" ? (
          <TaxonomyTab isAdmin={isAdmin} />
        ) : tab === "settings" ? (
          settingsSection ? (
            <FillerSettings section={settingsSection} />
          ) : (
            <FillerSettingsIndex />
          )
        ) : (
          <FillerCatalog isAdmin={isAdmin} />
        )}
      </div>
    </div>
  );
};

const FillerDescription = () => (
  <>
    Loomarr finds and prepares commercials, bumpers, and station IDs. Each channel picks what fits from the
    shared library.
  </>
);

export { FillerPage };

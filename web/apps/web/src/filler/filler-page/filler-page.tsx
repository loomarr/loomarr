import * as fillerApi from "@loomarr/api/endpoints/filler";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import { unwrap } from "@loomarr/api/unwrap";
import { formatRelative, pluralize } from "@loomarr/core/format";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { PoolHealth } from "@/components/loomarr/filler/pool-health";
import { WatchLine } from "@/components/loomarr/filler/watch-line";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { NavTabs } from "@/components/ui/nav-tabs";
import { useDocumentTitle } from "@/lib/use-document-title";
import { usePhoneWidth } from "@/lib/use-phone-width";
import { FillerCatalog } from "../filler-catalog";
import { FillerManage } from "../filler-manage";
import { FillerOverview } from "../filler-overview";
import { FillerQuality } from "../filler-quality";
import type { FillerSearch } from "../filler-search";
import { FillerSettings, FillerSettingsIndex } from "../filler-settings";
import { Incoming } from "../incoming";
import { SourcesTab } from "../sources-tab";
import { TaxonomyTab } from "../taxonomy-tab";
import type { FillerPageProps } from "./filler-page.type";

// FillerPage is the route-level composition root. It owns only state shared across destinations:
// navigation counts, installation readiness, the watch/health summary. Destination-specific behavior stays in its own module.
const FillerPage = ({ tab, settingsSection, sourceID }: FillerPageProps) => {
  useDocumentTitle("Filler");
  const navigate = useNavigate();
  const { isAdmin, isLoading: authLoading } = useAuth();
  const phone = usePhoneWidth();

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

  // Settings are admin-only. Members derive the same installation state from the
  // member-readable watch projection instead of probing a route they cannot access.
  const settings = settingsApi.useSettingsList({ query: { enabled: isAdmin } });
  const watch = unwrap(fillerApi.useFillerWatch().data, (body) => body);
  const fillerConfigured = isAdmin
    ? Boolean(unwrap(settings.data, (body) => body.features)?.filler)
    : watch
      ? watch.health !== "unconfigured"
      : undefined;
  const configurationPending = authLoading || (isAdmin ? settings.isPending : watch === undefined);
  const poolQuery = fillerApi.useFillerPool({ query: { enabled: tab === "library" } });
  const pool = unwrap(poolQuery.data, (body) => body);

  if (configurationPending && tab !== "settings") {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <FillerHeader />
        <div className="min-h-0 flex-1 p-6" role="status">
          <p className="text-muted-foreground text-sm">Loading filler…</p>
        </div>
      </div>
    );
  }

  if (!fillerConfigured && tab !== "settings") {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <FillerHeader />
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

  // The web mock's watch line: "3 of 4 sources on · 1,284 clips · 3 need you · checked 12m ago".
  // `held` is the clips waiting for review in Incoming, which is what "need you" counts.
  const statusLine = watch
    ? [
        `${watch.sourcesOn} of ${watch.sourcesTotal} sources on`,
        pluralize(watch.clips, "clip"),
        ...(watch.held > 0 ? [`${watch.held} need you`] : []),
        ...(watch.lastScanAt ? [`checked ${formatRelative(watch.lastScanAt)}`] : []),
      ].join(" · ")
    : "";

  return (
    <div className="flex h-full min-h-0 flex-col">
      <FillerHeader status={watch && <WatchLine status={statusLine} health={watch.health} />} />
      <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-auto p-6">
        {/* The web mock's three sections. Sources and Incoming open from the Manage hub, so their
            pages light Manage, as Settings and Tags already did. */}
        <NavTabs
          label="Filler sections"
          linkComponent={Link}
          tabs={[
            { id: "overview", label: "Overview", to: "/filler" },
            {
              id: "library",
              label: "Library",
              to: "/filler/library",
              search: catalogSearch,
              count: watch?.clips,
            },
            { id: "manage", label: "Manage", to: "/filler/manage" },
          ]}
          activeId={
            tab === "taxonomy" ||
            tab === "settings" ||
            tab === "sources" ||
            tab === "incoming" ||
            tab === "quality"
              ? "manage"
              : tab
          }
        />

        {/* Manage's children say where they return to, as the Settings index already did: the lit
            Manage tab scrolls away with the page on a phone (#1817, decided on #1908). */}
        {tab === "incoming" || tab === "sources" || tab === "taxonomy" || tab === "quality" ? (
          <Link
            to="/filler/manage"
            className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 self-start text-sm"
          >
            <ArrowLeft className="size-3.5" aria-hidden />
            Manage
          </Link>
        ) : null}

        {pool && tab === "library" ? <PoolHealth pool={pool} folded={phone} /> : null}

        {watch?.autoFetch?.stoppedBy === "catalog" && tab === "library" ? (
          <Card className="border-caution/40 bg-caution/5 flex flex-wrap items-center gap-3 p-4">
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">Automatic fetching is paused</p>
              <p className="text-muted-foreground mt-0.5 text-sm">
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
        ) : tab === "quality" && isAdmin ? (
          <FillerQuality />
        ) : tab === "sources" ? (
          <SourcesTab
            selectedSourceID={sourceID}
            onSelectSource={(selectedSourceID) =>
              navigate({
                to: "/filler/sources/$sourceId",
                params: { sourceId: selectedSourceID },
              })
            }
            onCloseSource={() => navigate({ to: "/filler/sources", replace: true })}
          />
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

// The header keeps the shared 24px page gutter at every width (frontend-design.md, "One page-edge
// contract"), so Filler's title sits where every other page's does.
const FillerHeader = ({ status }: { status?: ReactNode }) => (
  <PageHeader title="Filler" description={<FillerDescription />} status={status} />
);

export { FillerPage };

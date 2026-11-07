import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { Link } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { Button } from "@/components/ui/button";
import { StatusDot, type StatusTone } from "@loomarr/design-system";

type HubRow = {
  title: string;
  summary: string;
  tone: StatusTone;
  // A route to open; a row without one is informational.
  open?: {
    to: "/filler/sources" | "/filler/incoming" | "/filler/quality" | "/filler/taxonomy" | "/filler/settings";
  };
};

// FillerManage — the web mock's Manage tab: one list of the filler tools, each with a status dot,
// a one-line state and Open. Sources, Incoming and Quality are admin-only, as their pages are.
// Every summary comes from data the page already serves; none is guessed.
const FillerManage = () => {
  const { isAdmin } = useAuth();
  const watch = unwrap(fillerApi.useFillerWatch().data, (body) => body);
  const readiness = unwrap(fillerApi.useFillerReadiness().data, (body) => body);
  // The same page of the report the Quality queue opens on, so the two counts agree.
  const quality = unwrap(
    fillerApi.useFillerDefectReport(
      { unlabelled: true, limit: 1, offset: 0 },
      { query: { enabled: isAdmin } },
    ).data,
    (body) => body,
  );

  // "Need a choice" is the watch line's `held` (clips waiting for review in Incoming), so the hub
  // and the header's "N need you" are always the same number.
  const needsChoice = watch?.held ?? 0;
  const preparing = readiness?.pipeline.inProgress ?? 0;
  const untagged = readiness?.pool.untagged ?? 0;

  const rows: HubRow[] = [
    ...(isAdmin
      ? [
          {
            title: "Sources",
            // The mock also names the sources on. A source's `target` is its URL or folder
            // path, not a display name, so the names wait for one rather than printing paths.
            summary: watch ? `${watch.sourcesOn} of ${watch.sourcesTotal} on` : "Checking sources…",
            tone: (watch && watch.sourcesOn > 0 ? "ok" : "off") as StatusTone,
            open: { to: "/filler/sources" as const },
          },
          {
            title: "Incoming",
            summary:
              needsChoice > 0
                ? `${pluralize(needsChoice, "clip")} ${needsChoice === 1 ? "needs" : "need"} a choice`
                : `Nothing needs you · ${pluralize(preparing, "clip")} preparing`,
            tone: (needsChoice > 0 ? "warn" : "ok") as StatusTone,
            open: { to: "/filler/incoming" as const },
          },
          {
            title: "Quality",
            // The floor retires a failing copy on its own within a pass (#2036), so a count to
            // retire is information, not a warning.
            summary: quality
              ? `${quality.verdicts.fail} would be retired · ${pluralize(quality.total, "clip")} to label`
              : "Checking…",
            tone: (quality && quality.total > 0 ? "warn" : "ok") as StatusTone,
            open: { to: "/filler/quality" as const },
          },
        ]
      : []),
    {
      title: "Tags",
      summary: `Product, format, season and audience · ${
        untagged > 0 ? `${pluralize(untagged, "clip")} not tagged yet` : "every clip tagged"
      }`,
      tone: untagged > 0 ? "warn" : "ok",
      open: { to: "/filler/taxonomy" },
    },
    {
      title: "Settings",
      summary: "Folders, downloads, storage and matching defaults",
      tone: "off",
      ...(isAdmin ? { open: { to: "/filler/settings" as const } } : {}),
    },
  ];

  return (
    <section aria-label="Filler tools" className="border-border bg-card overflow-hidden rounded-lg border">
      <ul>
        {rows.map((row) => (
          <li
            key={row.title}
            className="border-border flex items-center gap-3 border-b px-4 py-3 last:border-b-0"
          >
            {/* Decorative: the summary beside it says the same thing in words. */}
            <StatusDot tone={row.tone} label="" size="compact" />
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{row.title}</p>
              <p className="text-muted-foreground mt-0.5 text-xs">{row.summary}</p>
            </div>
            {row.open ? (
              <Button
                size="sm"
                variant="outline"
                aria-label={`Open ${row.title}`}
                render={<Link to={row.open.to} />}
              >
                Open
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
};

export { FillerManage };

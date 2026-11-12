import * as fillerApi from "@loomarr/api/endpoints/filler";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { Action, Stack, StatusDot, type StatusTone, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";

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
  const taxonomy = unwrap(fillerApi.useListTaxonomy().data, (body) => body);
  // The same page of the report the Quality queue opens on, so the two counts agree.
  const quality = unwrap(
    fillerApi.useFillerDefectReport(
      { unlabelled: true, limit: 1, offset: 0 },
      { query: { enabled: isAdmin } },
    ).data,
    (body) => body,
  );

  // Each row repeats the number on the page it opens (#2733). "Need a choice" and "preparing" are
  // Incoming's own totals, which the header reads too; "not tagged" is the Tags page's count of
  // unclassified clips.
  const needsChoice = watch?.needsYou ?? 0;
  const preparing = watch?.preparing ?? 0;
  const untagged = taxonomy?.unclassifiedClips ?? 0;

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
          <Stack
            align="center"
            className="border-border border-b px-4 py-3 last:border-b-0"
            direction="row"
            gap="control"
            key={row.title}
            render="li"
          >
            {/* Decorative: the summary beside it says the same thing in words. */}
            <StatusDot tone={row.tone} label="" size="compact" />
            <div className="min-w-0 flex-1">
              <Text as="p" textRole="cardLabel">
                {row.title}
              </Text>
              <Text as="p" className="mt-0.5" textRole="hint" tone="secondary">
                {row.summary}
              </Text>
            </div>
            {row.open ? (
              <Action
                accessibilityRole="link"
                render={<Link aria-label={`Open ${row.title}`} to={row.open.to} />}
                variant="outline"
              >
                Open
              </Action>
            ) : null}
          </Stack>
        ))}
      </ul>
    </section>
  );
};

export { FillerManage };

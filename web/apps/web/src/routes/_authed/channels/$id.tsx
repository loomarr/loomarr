import { type ChannelDTO, type ChannelPolicy, channelsApi, toProblem } from "@loomarr/api";
import { channelNumber, formatEpgTime, pluralize } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/auth";
import { ChannelIdentityField, ChannelNav, type ChannelNavSection } from "@/channels";
import type { OnAirState } from "@/components/loomarr";
import { ChannelDangerZone, ChannelIconField, ErrorState, OnAirIndicator } from "@/components/loomarr";
import { useLoomarrEventListener } from "@/events";
import { ChannelFiller } from "@/filler";
import { useDocumentTitle } from "@/lib";
import { ChannelAdvanced } from "./-channel-advanced";
import { ChannelProgramming } from "./-channel-programming";

// Channel detail (§12). TWO AUDIENCES: the top answers a viewer's questions — is it on,
// what's playing, what's next, how full is it — in plain words. Editing (rename/renumber,
// the programming rules, pause/delete) is admin-only and lives inline (§7 PATCH). There is
// NO manual "rebuild" button: an edit auto-reconciles server-side and the page updates live
// via the `channel` SSE frame (§9 self-maintaining). The scheduler internals stay tucked in
// an admin-only "Advanced" disclosure so the viewer surface reads plainly.

// The tab/section ids — the closed set the `?section=` deep-link accepts. Shared by
// validateSearch (URL narrowing) and the component (the tab registry + which panel shows).
const SECTION_IDS = ["info", "programming", "filler", "advanced", "danger"] as const;
type SectionId = (typeof SECTION_IDS)[number];

// `section` is OPTIONAL so a plain link to the channel (from the list, the palette, the approval
// queue) needs no search param — it just lands on the default tab. Only a deep-link sets it.
type ChannelDetailSearch = { section?: SectionId };

type AirState = { dot: OnAirState; label: string; detail: string };

const airStateOf = (ch: ChannelDTO): AirState => {
  const pushed = Boolean(ch.tunarrId);
  if (ch.status === "detached") {
    return { dot: "off", label: "Off air", detail: "Loomarr no longer manages this channel." };
  }
  if (ch.status === "paused") {
    return {
      dot: "off",
      label: "Paused",
      detail: "Off air, but kept — resume it below whenever you like.",
    };
  }
  if (ch.status === "drifted") {
    return {
      dot: "reconciling",
      label: "On air (catching up)",
      detail: "Something changed in your library — Loomarr is bringing the lineup back in line.",
    };
  }
  if (ch.status === "live" && pushed) {
    return { dot: "live", label: "On air", detail: "Playing now in your TV guide." };
  }
  // building, or live-without-a-push (not yet broadcasting). Once Tunarr is connected it
  // goes live on its own — no manual step.
  return {
    dot: "reconciling",
    label: "Not on air yet",
    detail: pushed ? "Getting ready to broadcast." : "It'll go live automatically once Tunarr is connected.",
  };
};

const ChannelDetailScreen = () => {
  const { id } = Route.useParams();
  const navigate = Route.useNavigate();
  const { isAdmin } = useAuth();
  const queryClient = useQueryClient();

  const channel = channelsApi.useGetChannel(id);
  useDocumentTitle(channel.data?.status === 200 ? channel.data.data.name : undefined);
  // Now/next is Tunarr's guide (§6) — absent until the channel is pushed, which is normal,
  // not an error, so it retries-false and simply shows an honest "nothing yet".
  const nowNext = channelsApi.useChannelsNowNext({ query: { retry: false } });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: channelsApi.getGetChannelQueryKey(id) });
    void queryClient.invalidateQueries({ queryKey: channelsApi.getChannelsNowNextQueryKey() });
    // The saved pod preview (GET …/pods) lives under its own key, not the channel key, so
    // a reconcile that changes the break — including an applied filler edit — wouldn't
    // refresh it without this. The Filler section reads the saved break here.
    void queryClient.invalidateQueries({ queryKey: channelsApi.getPreviewChannelPodsQueryKey(id) });
  };

  // Live update: a background reconcile (this edit, the sweep, a title landing) emits a
  // `channel` frame — refresh so the page reflects it with no manual action.
  useLoomarrEventListener({ onChannel: () => invalidate() });

  // Edits save seamlessly, then auto-reconcile server-side. A light toast confirms; the
  // edit is the trigger, never a separate "apply"/"rebuild" step.
  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSuccess: () => {
        invalidate();
        toast.success("Saved");
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't save that change"),
    },
  });
  const del = channelsApi.useDeleteChannel({
    mutation: {
      onSuccess: () => {
        toast.success("Channel deleted");
        void navigate({ to: "/channels" });
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't delete the channel"),
    },
  });

  // The section registry drives BOTH the tab bar and which panel shows, so they can't drift.
  const sections: ChannelNavSection[] = [
    { id: "info", label: "Channel info" },
    { id: "programming", label: "Programming" },
    { id: "filler", label: "Filler" },
    { id: "advanced", label: "Advanced" },
    { id: "danger", label: "Danger zone" },
  ];

  // The active tab is URL-driven (`?section=`) — deep-linkable, shareable, back-button aware.
  // Absent (a plain link) → the default "info" tab. A click navigates, updating the URL.
  const { section } = Route.useSearch();
  const activeId: SectionId = section ?? "info";
  const selectSection = (sid: string) =>
    navigate({ to: "/channels/$id", params: { id }, search: { section: sid as SectionId } });

  if (channel.error) {
    return (
      <div className="p-6">
        <ErrorState error={channel.error} onRetry={() => channel.refetch()} />
      </div>
    );
  }
  const ch = channel.data?.status === 200 ? channel.data.data : undefined;
  if (!ch) return <p className="p-6 text-muted-foreground text-sm">Loading channel…</p>;

  const air = airStateOf(ch);
  const guide =
    nowNext.data?.status === 200 ? nowNext.data.data.channels?.find((c) => c.channelId === id) : undefined;

  // Count TITLES, not slots: slotCount includes commercial-break gaps (§10), so it would
  // read "12 of 15 shows ready" forever on a healthy channel with ad breaks. programCount +
  // pendingCount is the real title total; ready < total iff something is still acquiring.
  const ready = ch.programCount;
  const total = ch.programCount + ch.pendingCount;

  const savePolicy = (policy: ChannelPolicy) => update.mutate({ id, data: { policy } });

  // The identity fields commit through mutateAsync so they can distinguish success (adopt +
  // close) from failure (a 409 renumber surfaces inline on the field, editor stays open). The
  // hook-level onError toast still fires; the field also gets the rejection to show in place.
  const saveName = (name: string | number) =>
    update.mutateAsync({ id, data: { name: String(name) } }).then(() => undefined);
  const saveNumber = (number: string | number) =>
    update.mutateAsync({ id, data: { number: Number(number) } }).then(() => undefined);
  // The icon field's three "set" paths (TMDB suggestion, pasted URL) and its "clear" all
  // converge on this — the SAME channel update mutation identity/number use, `logo` is just
  // another field on it (§7 PATCH). Upload is the one path that doesn't call this directly
  // for the value (the raw multipart endpoint already set `logo` server-side); it still uses
  // this to trigger the shared invalidate.
  const saveLogo = (logo: string) => update.mutateAsync({ id, data: { logo } }).then(() => undefined);

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-border border-b px-6 py-4">
        <Link
          to="/channels"
          aria-label="Back to channels"
          className="cursor-pointer text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden />
        </Link>
        {/* Identity. Admins edit the name/number inline with explicit Save/Cancel per field
            (ChannelIdentityField); viewers see the plain heading. The mono channel number is
            the anchor glyph either way (§2.2 — machine data is mono). */}
        {isAdmin ? (
          <div className="flex flex-1 flex-wrap items-center gap-3">
            <ChannelIdentityField
              label="Channel name"
              value={ch.name}
              variant="title"
              validate={(v) => (v.trim().length === 0 ? "Give the channel a name." : undefined)}
              onSave={saveName}
            />
            <label className="ml-auto flex items-center gap-2 text-muted-foreground text-sm">
              <span className="font-mono uppercase tracking-wide">Ch</span>
              <ChannelIdentityField
                label="Channel number"
                value={ch.number}
                type="number"
                variant="compact"
                validate={(v) => {
                  const n = Number(v);
                  if (v.trim() === "" || !Number.isInteger(n) || n < 1) return "Use a whole number ≥ 1.";
                  return undefined;
                }}
                onSave={saveNumber}
              />
            </label>
          </div>
        ) : (
          <>
            <h1 className="font-semibold text-xl">{ch.name}</h1>
            <span className="ml-auto font-mono text-muted-foreground text-sm">
              Channel {channelNumber(ch.number)}
            </span>
          </>
        )}
      </header>

      {/* A HORIZONTAL tab bar under the header (admin only). Each tab is a section; clicking one
          shows ONLY that section's panel below and updates the URL (`?section=`) so it's deep-
          linkable. A viewer sees no bar — just the read-only Channel info. */}
      {isAdmin && <ChannelNav sections={sections} activeId={activeId} onSelect={selectSection} />}

      {/* One section at a time (tabs). The panel is centered at a comfortable width and scrolls
          if its own content is tall — but the page never grows into a long stack, so there's no
          scroll-to / scroll-spy machinery. `activeId` (from the URL) picks the panel. */}
      <div className="flex-1 overflow-auto">
        <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 p-6">
          {/* CHANNEL INFO — the read-only "is it working" at a glance. Always available (also the
              viewer's only panel). */}
          {(!isAdmin || activeId === "info") && (
            <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-5">
              <div className="flex items-center gap-3">
                <span className="font-mono text-3xl tabular-nums leading-none">
                  {channelNumber(ch.number)}
                </span>
                <div className="flex items-center gap-2">
                  <OnAirIndicator state={air.dot} />
                  {/* The status is the info panel's heading (also what the reachability check
                      keys on — a real heading proves the screen composed). */}
                  <h2 className="font-semibold text-lg">{air.label}</h2>
                </div>
              </div>
              <p className="text-muted-foreground text-sm">{air.detail}</p>

              <div className="border-border border-t pt-3">
                <NowNextRow label="Now playing" entry={guide?.now} live={air.dot === "live"} />
                <div className="my-2 border-border border-t" />
                <NowNextRow label="Up next" entry={guide?.next} live={air.dot === "live"} />
              </div>

              <p className="border-border border-t pt-3 text-sm">
                <span className="font-medium">{`${ready} of ${pluralize(total, "title")} ready`}</span>
                {ready < total && (
                  <span className="text-muted-foreground">
                    {" "}
                    — the rest fill in as titles arrive, so the channel never goes dark.
                  </span>
                )}
              </p>
            </section>
          )}

          {(!isAdmin || activeId === "info") && (
            <section className="rounded-lg border border-border bg-card p-5">
              <ChannelIconField channelId={id} logo={ch.logo} onSetLogo={saveLogo} isAdmin={isAdmin} />
            </section>
          )}

          {/* Admin editing panels — each shown only when its tab is active. Programming folds the
              old Lineup + Programming-rules + Refine tabs into one surface; Advanced renders its
              fields directly under a section shell; Filler keeps its internal sandbox. */}
          {isAdmin && activeId === "programming" && (
            <section className="rounded-lg border border-border p-5">
              <ChannelProgramming
                channelId={id}
                channelName={ch.name}
                lineup={ch.lineup ?? []}
                policy={ch.policy}
                onPolicyChange={savePolicy}
                onRefined={invalidate}
              />
            </section>
          )}

          {isAdmin && activeId === "filler" && <ChannelFiller channelId={id} policy={ch.policy} open />}

          {isAdmin && activeId === "advanced" && (
            <section className="flex flex-col gap-4 rounded-lg border border-border p-5">
              <div>
                <h2 className="font-semibold text-lg">Advanced</h2>
                <p className="text-muted-foreground text-sm">How this channel is built.</p>
              </div>
              <ChannelAdvanced channel={ch} />
            </section>
          )}

          {isAdmin && activeId === "danger" && (
            <ChannelDangerZone
              channelName={ch.name}
              status={ch.status}
              busy={update.isPending || del.isPending}
              onPause={() => update.mutate({ id, data: { status: "paused" } })}
              onResume={() => update.mutate({ id, data: { status: "building" } })}
              onDelete={({ purge }) => del.mutate({ id, params: { purge } })}
            />
          )}
        </div>
      </div>
    </div>
  );
};

// NowNextRow — a single "Now playing / Up next" line. An empty guide is the common, honest
// case (nothing pushed yet), rendered as a plain em-dash rather than a scary blank.
const NowNextRow = ({
  label,
  entry,
  live,
}: {
  label: string;
  entry?: { title: string; gap?: boolean; stopMs?: number };
  live: boolean;
}) => {
  const value = !entry
    ? live
      ? "Nothing scheduled right now"
      : "—"
    : entry.gap
      ? "A commercial break"
      : entry.title;
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className="text-muted-foreground text-sm">{label}</span>
      <span className="min-w-0 flex-1 truncate text-right text-sm">
        {value}
        {entry && !entry.gap && entry.stopMs ? (
          <span className="ml-2 font-mono text-muted-foreground text-xs">
            til {formatEpgTime(entry.stopMs)}
          </span>
        ) : null}
      </span>
    </div>
  );
};

const Route = createFileRoute("/_authed/channels/$id")({
  // The active tab is deep-linkable via `?section=`. Narrow an unknown/absent value to "info"
  // so a stale or hand-typed link lands on a valid tab rather than a blank panel.
  validateSearch: (search: Record<string, unknown>): ChannelDetailSearch => ({
    // Keep a valid section; drop anything else (absent → the component defaults to "info").
    section: SECTION_IDS.includes(search.section as SectionId) ? (search.section as SectionId) : undefined,
  }),
  component: ChannelDetailScreen,
});

export { Route };

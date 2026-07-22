import { type ChannelDTO, type ChannelPolicy, channelsApi, toProblem } from "@loomarr/api";
import { channelNumber, formatEpgTime, pluralize } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/auth";
import { ChannelIdentityField } from "@/channels";
import type { OnAirState } from "@/components/loomarr";
import {
  ChannelDangerZone,
  ChannelLineupEditor,
  ChannelPolicyFields,
  CollapsibleSection,
  ErrorState,
  OnAirIndicator,
  RefinePanel,
} from "@/components/loomarr";
import { useLoomarrEventListener } from "@/events";
import { ChannelFiller } from "@/filler";
import { cn } from "@/lib";
import { ChannelAdvanced } from "./-channel-advanced";

// Channel detail (§12). TWO AUDIENCES: the top answers a viewer's questions — is it on,
// what's playing, what's next, how full is it — in plain words. Editing (rename/renumber,
// the programming rules, pause/delete) is admin-only and lives inline (§7 PATCH). There is
// NO manual "rebuild" button: an edit auto-reconciles server-side and the page updates live
// via the `channel` SSE frame (§9 self-maintaining). The scheduler internals stay tucked in
// an admin-only "Advanced" disclosure so the viewer surface reads plainly.

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

  const ready = ch.programCount;
  const total = ch.slotCount;

  const savePolicy = (policy: ChannelPolicy) => update.mutate({ id, data: { policy } });

  // The identity fields commit through mutateAsync so they can distinguish success (adopt +
  // close) from failure (a 409 renumber surfaces inline on the field, editor stays open). The
  // hook-level onError toast still fires; the field also gets the rejection to show in place.
  const saveName = (name: string | number) =>
    update.mutateAsync({ id, data: { name: String(name) } }).then(() => undefined);
  const saveNumber = (number: string | number) =>
    update.mutateAsync({ id, data: { number: Number(number) } }).then(() => undefined);

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

      {/* Two-column: a sticky identity/at-a-glance rail beside the scrolling edit workbench.
          Stacks to one column below lg; for a viewer (no workbench) the rail is full-width. */}
      <div className="mx-auto w-full max-w-6xl flex-1 overflow-auto p-6">
        <div className={cn("grid gap-6", isAdmin && "lg:grid-cols-[minmax(280px,340px)_1fr]")}>
          {/* LEFT RAIL — channel identity + "is it working" at a glance. The inner wrapper is
              sticky on desktop so status/now-next stay in view while the tall workbench scrolls
              past; the danger zone lives here because it's channel lifecycle, next to identity,
              not buried at the bottom. The aside is the full-height grid cell (giving the
              sticky child room to travel); the wrapper is what actually sticks. */}
          <aside className="flex flex-col gap-5 lg:sticky lg:top-0 lg:self-start">
            {/* Status hero — the anchor: the mono channel number, the live dot, the honest
                one-liner. Larger than the rest so the eye lands here first. */}
            <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-5">
              <div className="flex items-center gap-3">
                <span className="font-mono text-3xl tabular-nums leading-none">
                  {channelNumber(ch.number)}
                </span>
                <div className="flex items-center gap-2">
                  <OnAirIndicator state={air.dot} />
                  <span className="font-semibold text-lg">{air.label}</span>
                </div>
              </div>
              <p className="text-muted-foreground text-sm">{air.detail}</p>

              <div className="border-border border-t pt-3">
                <NowNextRow label="Now playing" entry={guide?.now} live={air.dot === "live"} />
                <div className="my-2 border-border border-t" />
                <NowNextRow label="Up next" entry={guide?.next} live={air.dot === "live"} />
              </div>

              {/* How full — plain words, no "slots/flex/filler" jargon. */}
              <p className="border-border border-t pt-3 text-sm">
                <span className="font-medium">{`${ready} of ${pluralize(total, "show")} ready`}</span>
                {ready < total && (
                  <span className="text-muted-foreground">
                    {" "}
                    — the rest fill in as clips arrive, so the channel never goes dark.
                  </span>
                )}
              </p>
            </section>

            {isAdmin && (
              <ChannelDangerZone
                channelName={ch.name}
                status={ch.status}
                busy={update.isPending || del.isPending}
                onPause={() => update.mutate({ id, data: { status: "paused" } })}
                onResume={() => update.mutate({ id, data: { status: "building" } })}
                onDelete={({ purge }) => del.mutate({ id, params: { purge } })}
              />
            )}
          </aside>

          {/* RIGHT COLUMN — the editing workbench (admin only). Refine → Lineup → Programming
              rules → Filler → Advanced, each a section with its own header + breathing room. */}
          {isAdmin && (
            <div className="flex min-w-0 flex-col gap-6">
              {/* Refine is the headline admin action — describe a change, review the diff, apply. */}
              <RefinePanel
                channelId={id}
                channelName={ch.name}
                current={(ch.lineup ?? []).map((entry) => ({
                  name: entry.name,
                  year: entry.year,
                  key: entry.key,
                }))}
                onApplied={invalidate}
              />

              {/* The direct, manual path alongside Refine: add/remove/reorder titles by hand.
                  No diff, no rebuild — each edit commits and reconciles immediately (§9). */}
              <ChannelLineupEditor channelId={id} lineup={ch.lineup ?? []} />

              {/* Programming rules — a settings-heavy section, so it's collapsible: opens calm,
                  you expand it to tune ordering / audience / era / no-repeat. */}
              <CollapsibleSection
                title="Programming rules"
                description="How this channel picks and orders what plays. Defaults follow your global settings."
              >
                <ChannelPolicyFields policy={ch.policy} onChange={savePolicy} />
              </CollapsibleSection>

              {/* Filler — its own collapsible sandbox (§10/§12); starts closed. */}
              <ChannelFiller channelId={id} policy={ch.policy} />

              {/* Advanced — the scheduler internals, collapsed so the surface above stays plain. */}
              <CollapsibleSection title="Advanced" description="How this channel is built.">
                <ChannelAdvanced channel={ch} />
              </CollapsibleSection>
            </div>
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
  component: ChannelDetailScreen,
});

export { Route };

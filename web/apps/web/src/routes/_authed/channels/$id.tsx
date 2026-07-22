import { type ChannelDTO, type ChannelPolicy, channelsApi, toProblem } from "@loomarr/api";
import { formatEpgTime, pluralize } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { ArrowLeft, ChevronDown } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/auth";
import type { OnAirState } from "@/components/loomarr";
import {
  ChannelDangerZone,
  ChannelLineupEditor,
  ChannelPolicyFields,
  ErrorState,
  OnAirIndicator,
  RefinePanel,
} from "@/components/loomarr";
import { Input } from "@/components/ui";
import { useLoomarrEventListener } from "@/events";
import { ChannelFiller } from "@/filler";
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
  const [showAdvanced, setShowAdvanced] = useState(false);

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
        {/* Rename / renumber in place (admin) — commit on blur (§7 PATCH), so typing a name
            doesn't PATCH every keystroke. Non-admins see the plain heading. */}
        {isAdmin ? (
          <>
            <Input
              aria-label="Channel name"
              defaultValue={ch.name}
              disabled={update.isPending}
              className="h-auto max-w-xs border-transparent bg-transparent px-1 font-semibold text-xl hover:border-input focus:border-input"
              onBlur={(e) => {
                const name = e.target.value.trim();
                if (name && name !== ch.name) update.mutate({ id, data: { name } });
              }}
            />
            <label className="ml-auto flex items-center gap-1.5 font-mono text-muted-foreground text-sm">
              Ch
              <Input
                aria-label="Channel number"
                type="number"
                min={1}
                defaultValue={ch.number}
                disabled={update.isPending}
                className="h-8 w-16 px-2 text-center"
                onBlur={(e) => {
                  const number = Number(e.target.value);
                  if (Number.isFinite(number) && number >= 1 && number !== ch.number) {
                    update.mutate({ id, data: { number } });
                  }
                }}
              />
            </label>
          </>
        ) : (
          <>
            <h1 className="font-semibold text-xl">{ch.name}</h1>
            <span className="ml-auto font-mono text-muted-foreground text-sm">Channel {ch.number}</span>
          </>
        )}
      </header>

      <div className="mx-auto flex w-full max-w-2xl flex-1 flex-col gap-6 overflow-auto p-6">
        {/* Status — the honest one-liner. */}
        <section className="flex items-start gap-3">
          <OnAirIndicator state={air.dot} className="mt-1.5" />
          <div>
            <p className="font-semibold text-lg">{air.label}</p>
            <p className="text-muted-foreground text-sm">{air.detail}</p>
          </div>
        </section>

        {/* What's on — a viewer's first question. */}
        <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
          <NowNextRow label="Now playing" entry={guide?.now} live={air.dot === "live"} />
          <div className="border-border border-t" />
          <NowNextRow label="Up next" entry={guide?.next} live={air.dot === "live"} />
        </section>

        {/* How full — plain words, no "slots/flex/filler" jargon. */}
        <section className="flex flex-col gap-1">
          <p className="text-sm">
            <span className="font-medium">{`${ready} of ${pluralize(total, "show")} ready`}</span>
            {ready < total && (
              <span className="text-muted-foreground">
                {" "}
                — the rest fill in with clips as they arrive, so the channel never goes dark.
              </span>
            )}
          </p>
        </section>

        {/* Admin editing + internals, below the viewer surface. */}
        {isAdmin && (
          <>
            {/* Refine is the headline admin action — describe a change, review the diff,
                apply — so it sits above the programming-rules editor rather than beside
                the internals. */}
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

            {/* The direct, manual path alongside Refine: add/remove/reorder titles by
                hand. No diff to review and no rebuild step — each edit commits and
                reconciles immediately (§9), same as every other admin control here. */}
            <ChannelLineupEditor channelId={id} lineup={ch.lineup ?? []} />

            <section className="flex flex-col gap-3 rounded-lg border border-border p-4">
              <div>
                <h2 className="font-semibold text-lg">Programming rules</h2>
                <p className="text-muted-foreground text-sm">
                  How this channel picks and orders what plays. Anything left on a default just follows your
                  global settings.
                </p>
              </div>
              <ChannelPolicyFields policy={ch.policy} onChange={savePolicy} />
            </section>

            {/* Filler — choose and preview the channel's ad breaks. A live draft sandbox:
                shape the theme + pin/exclude clips, watch the break re-assemble, then
                apply. Sits between the programming rules and the internals because it's
                part of shaping the channel, not a scheduler detail (§10, §12). */}
            <ChannelFiller channelId={id} policy={ch.policy} />

            {/* Advanced — the scheduler internals (relaxations and the Tunarr id),
                collapsed so the surface above stays plain. */}
            <section className="rounded-lg border border-border">
              <button
                type="button"
                onClick={() => setShowAdvanced((v) => !v)}
                aria-expanded={showAdvanced}
                className="flex w-full cursor-pointer items-center gap-2 px-4 py-3 text-left text-muted-foreground text-sm transition-colors hover:bg-static-800 hover:text-foreground"
              >
                <span className="font-medium">Advanced</span>
                <span className="text-static-400 text-xs">how this channel is built</span>
                <ChevronDown
                  className={`ml-auto size-4 transition-transform ${showAdvanced ? "rotate-180" : ""}`}
                  aria-hidden
                />
              </button>
              {showAdvanced && <ChannelAdvanced channel={ch} />}
            </section>

            <ChannelDangerZone
              channelName={ch.name}
              status={ch.status}
              busy={update.isPending || del.isPending}
              onPause={() => update.mutate({ id, data: { status: "paused" } })}
              onResume={() => update.mutate({ id, data: { status: "building" } })}
              onDelete={({ purge }) => del.mutate({ id, params: { purge } })}
            />
          </>
        )}
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

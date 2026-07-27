import { channelsApi, toProblem } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { MoreVertical, Pause, Pencil, Play, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui";
import { useDeleteConfirm } from "../use-delete-confirm";
import type { ChannelRowMenu as ChannelRowMenuProps } from "./channel-row-menu.type";

// swallow — stop a click from bubbling to the row's <Link> (the whole card navigates). Every
// interactive element in this menu lives INSIDE that link, so without this, opening the menu
// or typing the delete-confirm name would also route to the channel. preventDefault covers
// the Link's own navigation; stopPropagation covers any ancestor handler.
const swallow = (e: React.MouseEvent) => {
  e.preventDefault();
  e.stopPropagation();
};

// ChannelRowMenu — the per-row ⋮ menu on the channels list: pause/resume and delete without
// opening the channel. Pause/resume is reversible → a single click. Delete is irreversible →
// a two-step confirm (arm, then execute) via the shared useDeleteConfirm hook — the SAME gate
// the detail page's ChannelDangerZone uses, so the confirm flow lives in one place.
//
// Self-contained (no dropdown primitive in the kit, no new dep): a trigger + a floating panel
// over a full-bleed backdrop that closes on an outside click — which also keeps the outside-
// click handling off the document and away from the row link.
const ChannelRowMenu = ({ channel }: ChannelRowMenuProps) => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const { confirming, arm, reset: resetConfirm } = useDeleteConfirm();

  const paused = channel.status === "paused";

  const invalidate = () =>
    void queryClient.invalidateQueries({ queryKey: channelsApi.getListChannelsQueryKey() });

  const close = () => {
    setOpen(false);
    resetConfirm();
  };

  const update = channelsApi.useUpdateChannel({
    mutation: {
      onSuccess: () => {
        invalidate();
        toast.success(paused ? "Channel resumed" : "Channel paused");
        close();
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't update the channel"),
    },
  });

  const del = channelsApi.useDeleteChannel({
    mutation: {
      onSuccess: () => {
        invalidate();
        toast.success("Channel deleted");
        close();
      },
      onError: (e) => toast.error(toProblem(e).title ?? "Couldn't delete the channel"),
    },
  });

  const busy = update.isPending || del.isPending;

  // No handlers on this wrapper div — each interactive child (the trigger, the backdrop, the
  // menu items, the confirm input) swallows its own click, so the wrapper stays a plain
  // static element (a11y: static elements shouldn't carry event handlers).
  return (
    <div className="relative shrink-0">
      <Button
        variant="ghost"
        size="icon"
        className="size-8"
        aria-label={`Actions for ${channel.name}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={(e) => {
          swallow(e);
          setOpen((v) => !v);
        }}
      >
        <MoreVertical className="size-4" aria-hidden />
      </Button>

      {open && (
        <>
          {/* Backdrop: an outside click closes the menu. Full-bleed + fixed so a click
              anywhere lands here first; it swallows so the click never reaches the row link. */}
          <button
            type="button"
            aria-hidden
            tabIndex={-1}
            className="fixed inset-0 z-40 cursor-default"
            onClick={(e) => {
              swallow(e);
              close();
            }}
          />
          <div
            role="menu"
            // `left-0` (not `right-0`): the trigger sits at the RIGHT edge of a narrow rail,
            // so a right-anchored panel opens leftward past the rail and gets clipped by the
            // page edge. The mock clamps it the same way — `max(8, railW - 242)` — i.e. the
            // menu is pinned inside the viewport rather than to the trigger.
            className="absolute left-0 z-50 mt-1 flex w-59 flex-col gap-0.5 rounded-lg border border-border bg-popover p-1.5 shadow-[0_14px_36px_rgba(0,0,0,0.6)]"
          >
            {/* Edit channel — FIRST, and the reason the menu is worth opening: the row's own
                click target opens the channel too, but a menu whose only options are Pause and
                Delete reads as a destructive-actions menu. The mock leads with Edit. */}
            <button
              type="button"
              role="menuitem"
              onClick={(e) => {
                swallow(e);
                close();
                void navigate({ to: "/channels/$id", params: { id: channel.id } });
              }}
              className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors hover:bg-static-700 hover:text-static-0"
            >
              <Pencil className="size-4 text-static-400" aria-hidden />
              Edit channel
            </button>

            {/* Pause / Resume — reversible, single click. */}
            <button
              type="button"
              role="menuitem"
              disabled={busy}
              onClick={(e) => {
                swallow(e);
                update.mutate({ id: channel.id, data: { status: paused ? "building" : "paused" } });
              }}
              className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors hover:bg-static-800 disabled:opacity-50"
            >
              {paused ? <Play className="size-4" aria-hidden /> : <Pause className="size-4" aria-hidden />}
              {paused ? "Resume" : "Pause"}
            </button>

            {/* Delete — a FULL removal (purge: the channel leaves the list AND Tunarr), so a
                deleted row actually disappears rather than lingering as a "detached" record.
                Irreversible → a typed-confirm gate (the exact name), same as ChannelDangerZone. */}
            {!confirming ? (
              <button
                type="button"
                role="menuitem"
                disabled={busy}
                onClick={(e) => {
                  swallow(e);
                  arm();
                }}
                className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-onair-300 text-sm transition-colors hover:bg-onair-tint-15 disabled:opacity-50"
              >
                <Trash2 className="size-4" aria-hidden />
                Delete…
              </button>
            ) : (
              <div className="flex flex-col gap-2 rounded border border-onair-tint-15 bg-onair-tint-15 p-2">
                <p className="text-xs">Delete {channel.name} for good? This can't be undone.</p>
                <div className="flex items-center gap-2">
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={busy}
                    onClick={(e) => {
                      swallow(e);
                      // purge: a Delete from the LIST should remove the channel outright, not
                      // leave a detached record behind (the maintainer's choice). The detail-
                      // page danger zone still offers detach-vs-purge for the finer distinction.
                      del.mutate({ id: channel.id, params: { purge: true } });
                    }}
                  >
                    Delete
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={(e) => {
                      swallow(e);
                      resetConfirm();
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
};

export { ChannelRowMenu };

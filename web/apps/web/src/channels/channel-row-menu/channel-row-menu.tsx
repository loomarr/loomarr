import { channelsApi, toProblem } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { MoreVertical, Pause, Play, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { Button, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
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
// a typed-confirm (the exact channel name), the SAME gate the detail page's ChannelDangerZone
// uses, so the list can't delete a channel more casually than the danger zone does.
//
// Self-contained (no dropdown primitive in the kit, no new dep): a trigger + a floating panel
// over a full-bleed backdrop that closes on an outside click — which also keeps the outside-
// click handling off the document and away from the row link.
const ChannelRowMenu = ({ channel }: ChannelRowMenuProps) => {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [typed, setTyped] = useState("");

  const paused = channel.status === "paused";

  const invalidate = () =>
    void queryClient.invalidateQueries({ queryKey: channelsApi.getListChannelsQueryKey() });

  const close = () => {
    setOpen(false);
    setConfirming(false);
    setTyped("");
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
  const canDelete = typed === channel.name;

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
            className="absolute right-0 z-50 mt-1 flex w-64 flex-col gap-1 rounded-md border border-border bg-popover p-1.5 shadow-lg"
          >
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
                  setConfirming(true);
                }}
                className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-left text-onair-300 text-sm transition-colors hover:bg-onair-tint-15 disabled:opacity-50"
              >
                <Trash2 className="size-4" aria-hidden />
                Delete…
              </button>
            ) : (
              <div className="flex flex-col gap-2 rounded border border-onair-tint-15 bg-onair-tint-15 p-2">
                <Label htmlFor={`del-${channel.id}`} className="text-xs">
                  {`Type "${channel.name}" to delete`}
                </Label>
                <Input
                  id={`del-${channel.id}`}
                  value={typed}
                  autoComplete="off"
                  disabled={busy}
                  className={cn("h-8", "bg-background/60")}
                  onClick={swallow}
                  onChange={(e) => setTyped(e.target.value)}
                />
                <div className="flex items-center gap-2">
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={!canDelete || busy}
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
                      setConfirming(false);
                      setTyped("");
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

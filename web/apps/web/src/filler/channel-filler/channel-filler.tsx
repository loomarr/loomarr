import { pluralize } from "@loomarr/core";
import { Link } from "@tanstack/react-router";
import { CollapsibleSection, PodTimeline } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import { useChannelFillerDraft } from "../use-channel-filler-draft";
import type { ChannelFillerProps } from "./channel-filler.type";
import { FillerClipList } from "./filler-clip-list";
import { FillerCriteria } from "./filler-criteria";
import { useFillerCatalog } from "./use-filler-catalog";

// ChannelFiller — the per-channel filler sandbox (§10, §12), the Filler section on the
// channel page. Three parts over one live draft: the THEME criteria (era/audience/
// categories/kinds), the OVERRIDES (pin "always include" / exclude "never include"), and a
// live PREVIEW of the actual break the draft would air. Editing anything re-assembles the
// break in front of you through the SAME assembler reconcile uses — nothing saved until
// Apply, which commits the draft to policy.filler and lets reconcile take over (seamless
// for the effect; draft/apply for the authoring, the one deliberate §10 exception).
const ChannelFiller = ({ channelId, policy, open, onOpenChange, className }: ChannelFillerProps) => {
  const { draft, setDraft, preview, isPreviewing, previewError, isDirty, apply, isApplying, discard } =
    useChannelFillerDraft(channelId, policy);

  // A small shared catalog so pinned/excluded ids can render as names, and so pin can't
  // offer a clip already excluded (and vice-versa). Cheap: a single filler list read.
  const { resolve } = useFillerCatalog();

  const entries = preview?.entries ?? [];
  const pinned = draft.pinned ?? [];
  const excluded = draft.excluded ?? [];

  return (
    // A collapsible section (starts closed): the sandbox is heavy, so it opens only when you
    // come to tune the breaks. The catalog link moves into the body (a Link can't live in the
    // header button). A trailing "unsaved" dot on the header signals a dirty draft while closed.
    <CollapsibleSection
      title="Filler"
      description="The commercials, bumpers, and station IDs that play between shows."
      open={open}
      onOpenChange={onOpenChange}
      className={className}
      trailing={
        isDirty ? (
          <span className="flex items-center gap-1.5 text-suggest-300 text-xs">
            <span className="size-2 rounded-full bg-suggest" aria-hidden />
            Unsaved
          </span>
        ) : undefined
      }
    >
      <div className="flex flex-col gap-5">
        <p className="text-muted-foreground text-sm">
          Tweak and watch the break update below — nothing saves until you apply. Clips come from your{" "}
          <Link to="/filler" className="text-signal underline-offset-2 hover:underline">
            filler catalog
          </Link>
          .
        </p>

        <FillerCriteria selection={draft} onChange={setDraft} disabled={isApplying} />

        <div className="flex flex-col gap-4 border-border border-t pt-5">
          <FillerClipList
            label="Always include"
            hint="Pin specific clips so every break can use them, ahead of the theme."
            ids={pinned}
            onChange={(next) => setDraft({ ...draft, pinned: next })}
            resolve={resolve}
            disabled={isApplying}
            excludeIds={excluded}
          />
          <FillerClipList
            label="Never include"
            hint="Block clips you don't want on this channel — they win over a pin."
            ids={excluded}
            onChange={(next) => setDraft({ ...draft, excluded: next })}
            resolve={resolve}
            disabled={isApplying}
            excludeIds={pinned}
          />
        </div>

        {/* The live break — the ground-truth preview of the current draft. */}
        <div className="flex flex-col gap-2 border-border border-t pt-5">
          <p className="font-medium text-sm">This channel's break</p>
          {previewError ? (
            <p className="text-onair-300 text-sm">
              Couldn't assemble a preview for this selection. Adjust the criteria and try again.
            </p>
          ) : entries.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {isPreviewing ? (
                "Assembling preview…"
              ) : (
                <>
                  No clips match this selection yet — breaks fall back to the bumper card. Loosen the theme
                  above, or{" "}
                  <Link to="/filler" className="text-signal underline-offset-2 hover:underline">
                    add and tag clips
                  </Link>{" "}
                  in your filler catalog.
                </>
              )}
            </p>
          ) : (
            <div className={cn("transition-opacity", isPreviewing && "opacity-60")}>
              <PodTimeline entries={entries} matchLevel={preview?.matchLevel} />
              <p className="mt-2 text-muted-foreground text-sm">
                {`${pluralize(entries.length, "clip")} in this break, assembled exactly as the channel builds it.`}
              </p>
            </div>
          )}
        </div>

        {/* Apply / Discard — only offered when the draft differs from what's saved. Apply is
          the seamless commit; Discard throws the draft away. */}
        {isDirty && (
          <div className="flex items-center gap-2 border-border border-t pt-4">
            <Button variant="suggest" size="sm" disabled={isApplying} onClick={apply}>
              {isApplying ? "Applying…" : "Apply filler"}
            </Button>
            <Button variant="ghost" size="sm" disabled={isApplying} onClick={discard}>
              Discard
            </Button>
            <span className="ml-auto text-muted-foreground text-xs">Unsaved changes</span>
          </div>
        )}
      </div>
    </CollapsibleSection>
  );
};

export { ChannelFiller };

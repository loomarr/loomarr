import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { Rating } from "@loomarr/api/models/rating";
import type { SeasonNotKept } from "@loomarr/api/models/seasonNotKept";
import { Action, Badge, Card, Icon, icons, Surface, Text } from "@loomarr/design-system";
import { useId, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { useAppForm } from "@/lib/app-form";
import { episodeSelectionLabel } from "../episode-selection-label";
import { ProposalEdit } from "../proposal-edit";
import type { ApprovalQueueItemProps } from "./approval-queue-item.type";

// seasonWindowLabel renders a series' airing season window (§8) as a human chip so a
// reviewer can see "classic X → Seasons 1–10" before approving. null = unbounded.
const seasonWindowLabel = (min?: number, max?: number): string | null => {
  const lo = min ?? 0;
  const hi = max ?? 0;
  if (lo <= 0 && hi <= 0) return null;
  if (lo > 0 && hi > 0) return lo === hi ? `Season ${lo}` : `Seasons ${lo}–${hi}`;
  if (lo > 0) return `From season ${lo}`;
  return `Through season ${hi}`;
};

// regionName is a country code's English name ("GB" → "United Kingdom"), or the code itself.
const regionName = (country: string): string => {
  try {
    return new Intl.DisplayNames(["en"], { type: "region" }).of(country) ?? country;
  } catch {
    return country;
  }
};

// seasonNotKeptLabel says which holidays aren't on the household's holiday calendar (#2030 decision
// 38). It names the calendar, not "your country": a country Loomarr has no holidays for reads the
// US calendar, and an undated holiday is on no calendar at all.
const seasonNotKeptLabel = ({ country, holidays }: SeasonNotKept): string => {
  const names = new Intl.ListFormat("en", { type: "conjunction" }).format(holidays);
  const one = holidays.length === 1;
  return `${names} ${one ? "isn't" : "aren't"} on your holiday calendar (${regionName(country)}), so this channel won't follow ${
    one ? "its season" : "their seasons"
  }.`;
};

// withAudience joins the title edits with a raised audience limit (#3012). The title editor and
// the Raise button each change their own half; no half left means no edit at all.
const withAudience = (
  edit: ApprovalEditDTO | undefined,
  audience: Rating | undefined,
): ApprovalEditDTO | undefined => {
  const titles =
    edit?.drop?.length || edit?.add?.length || edit?.note?.trim()
      ? { drop: edit.drop, add: edit.add, note: edit.note }
      : undefined;
  return audience ? { ...titles, audience } : titles;
};

const PickRow = ({
  item,
  kind,
  renderFeedback,
}: {
  item: ProposalItem;
  kind: "lineup" | "acquire";
  renderFeedback?: ApprovalQueueItemProps["renderFeedback"];
}) => {
  const window = seasonWindowLabel(item.seasonMin, item.seasonMax);
  return (
    <li className="border-border bg-card flex flex-wrap items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm">
      <span className="font-medium">{item.name}</span>
      {item.year ? <span className="text-static-400 font-mono text-xs">{item.year}</span> : null}
      <Badge shape="square" tone={kind === "lineup" ? "lock" : "tune"}>
        {kind === "lineup" ? "In library" : "Will acquire"}
      </Badge>
      {window && (
        <Badge shape="square" tone="tune">
          {window}
        </Badge>
      )}
      {episodeSelectionLabel(item) && (
        <Badge shape="square" tone="suggest">
          {episodeSelectionLabel(item)}
        </Badge>
      )}
      {renderFeedback && <div className="ml-auto">{renderFeedback(item)}</div>}
    </li>
  );
};

// ApprovalQueueItem — the admin approval row (§3, §7 approval gate). One glance: what
// was asked, by whom, how many acquisitions it implies; two actions: approve or deny.
// A "Show picks" toggle reveals the grounded lineup (titles, in-library vs. acquire,
// season windows) so an admin can REVIEW what they're approving before it goes live —
// otherwise the gate is blind. `approving` disables both and spins; `denied` shows the
// reason. Members never see this — the gate is admin-only (§11), enforced server-side.
const ApprovalQueueItem = ({
  title,
  requestedBy,
  summary,
  outlook,
  acquisitions,
  status = "pending",
  denyReason,
  lineup,
  acquisitionItems,
  alternates,
  episodeSelectionPreview,
  refused,
  audienceLimit,
  raiseAudienceTo,
  seasonNotKept,
  edit,
  onEdit,
  renderFeedback,
  onApprove,
  onDeny,
  error,
  flat = false,
}: ApprovalQueueItemProps) => {
  const busy = status === "approving";
  const denied = status === "denied";
  const [open, setOpen] = useState(false);
  const [denying, setDenying] = useState(false);
  // The reason is optional: blank denies without one.
  const form = useAppForm({
    defaultValues: { reason: "" },
    onSubmit: ({ value }) => onDeny?.(value.reason.trim() || undefined),
  });
  const reasonId = useId();
  const hasPicks = (lineup?.length ?? 0) + (acquisitionItems?.length ?? 0) > 0;
  const canRaise = raiseAudienceTo !== undefined && onEdit !== undefined;
  const raising = canRaise && edit?.audience !== undefined;

  return (
    <Card gap={12} {...(flat ? { backgroundColor: "transparent", borderWidth: 0 } : { padding: 16 })}>
      {/* On a phone the actions go under the pitch; beside it they left it a word per line (#1785). */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:gap-4">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-medium">{title}</p>
            {typeof acquisitions === "number" && acquisitions > 0 && (
              <Badge shape="square" tone="tune">{`${acquisitions} to acquire`}</Badge>
            )}
            {/* A pending edit changes what this row's Approve does, and keeps it out of bulk. */}
            {edit && (
              <Badge shape="square" tone="suggest">
                Edited
              </Badge>
            )}
            {denied && (
              <Badge shape="square" tone="onair">
                Denied
              </Badge>
            )}
          </div>
          {summary && <p className="text-muted-foreground mt-1 text-sm">{summary}</p>}
          {requestedBy && (
            <p className="text-static-400 mt-1 font-mono text-xs">{`Requested by ${requestedBy}`}</p>
          )}
          {denied && denyReason && <p className="text-onair-300 mt-1 text-sm">{denyReason}</p>}
        </div>

        {!denied && !denying && (
          <div className="flex shrink-0 gap-2">
            <Action disabled={busy} icon="close" onPress={() => setDenying(true)} variant="outline">
              Deny
            </Action>
            <Action aria-busy={busy} disabled={busy} icon={busy ? "loading" : "success"} onPress={onApprove}>
              Approve
            </Action>
          </div>
        )}
      </div>

      {/* ⚠ On the row, not in a page-level banner (#1430): a failed approve/deny belongs beside
          the card it failed for, not somewhere a reviewer scanning other rows has to go find. */}
      {error != null && <ErrorState error={error} />}

      {/* Deny arms this row rather than firing straight away. The reason is optional —
          requiring one would turn every decline into a chore — but offering it is what
          stops a member from re-submitting the same intent, having learned nothing. */}
      {denying && (
        <form
          className="border-border flex flex-col gap-2 border-t pt-2"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            void form.handleSubmit();
          }}
        >
          <form.AppField name="reason">
            {(field) => (
              <field.TextField
                autoFocus
                disabled={busy}
                id={reasonId}
                label="Why not? Optional: the requester sees this."
                placeholder="e.g. over the acquisition cap this week, ask again Monday"
              />
            )}
          </form.AppField>
          <div className="flex justify-end gap-2">
            <Action
              disabled={busy}
              onPress={() => {
                setDenying(false);
                form.reset();
              }}
              variant="ghost"
            >
              Cancel
            </Action>
            <Action disabled={busy} icon="close" onPress={() => void form.handleSubmit()} variant="outline">
              Deny
            </Action>
          </div>
        </form>
      )}

      {/* ⚠ OUTSIDE the picks disclosure, and above it. What a reviewer is approving changed:
          these titles were proposed and will not play. Before this, the card offered them
          silently and the §4 gate dropped them after approval — so an admin authorised seven
          titles, got five, and nothing anywhere said which two or why (#259). A fact that
          changes the meaning of the button is not detail behind a toggle. */}
      {refused && refused.length > 0 && (
        // A warning takes `caution` (#1909 Q5); amber is focus and the primary action.
        <div className="border-caution-tint-40 bg-caution-tint-8 flex flex-col gap-1.5 rounded-md border px-3 py-2">
          <p className="flex items-center gap-1.5 text-sm">
            <Icon decorative glyph={icons.risk} size="compact" tone="warning" />
            <span className="font-medium">
              {refused.length} {refused.length === 1 ? "title was" : "titles were"} left out
            </span>
          </p>
          <ul className="flex flex-col gap-0.5">
            {refused.map((r) => (
              <li key={`refused-${r.item.name}-${r.item.tmdbId ?? ""}`} className="text-sm">
                <span>{r.item.name}</span>{" "}
                <span className="text-muted-foreground">
                  {/* The reason is spelled out rather than shown as a code: the reviewer's
                      next action (raise the limit, or accept the shorter lineup) depends on
                      knowing it was the audience rule and not a missing file. */}
                  — rated {r.item.officialRating || "unknown"}
                </span>
              </li>
            ))}
          </ul>
          <p className="text-muted-foreground text-xs">
            {raising
              ? `Approving raises this channel's audience limit to ${edit?.audience}, so titles rated up to ${edit?.audience} are included.`
              : `${
                  audienceLimit
                    ? `This channel's audience limit is ${audienceLimit}, so anything rated higher is left out.`
                    : "This channel has an audience limit, so anything rated higher is left out."
                } Approving adds everything else.${
                  canRaise ? "" : " To include these, raise the audience limit after the channel is created."
                }`}
          </p>
          {/* #3012: one button, the lowest limit that lets these in (named by the server). It
              rides the approval like any other edit, so it applies when the channel is created;
              pressing it again takes it back. */}
          {canRaise && (
            <Surface alignItems="flex-start" backgroundColor="transparent" borderWidth={0}>
              <Action
                aria-pressed={raising}
                disabled={busy}
                onPress={() => onEdit?.(withAudience(edit, raising ? undefined : raiseAudienceTo))}
                selected={raising}
                variant="outline"
              >
                {`Raise to ${raiseAudienceTo}`}
              </Action>
            </Surface>
          )}
        </div>
      )}

      {/* Beside the refusals for the same reason: approving gets a channel that doesn't follow the
          season that was asked for (#2030 decision 38). */}
      {seasonNotKept && seasonNotKept.holidays.length > 0 && (
        <Surface
          backgroundColor="stateWarningSurface"
          borderColor="stateWarning"
          borderRadius="control"
          borderWidth={1}
          padding={12}
        >
          <Text textRole="hint">{seasonNotKeptLabel(seasonNotKept)}</Text>
        </Surface>
      )}

      {outlook?.(() => setOpen(true))}

      {hasPicks && (
        <div className="border-border border-t pt-2">
          <Action
            aria-expanded={open}
            icon={open ? "showLess" : "showMore"}
            onPress={() => setOpen((v) => !v)}
            style={{ alignSelf: "flex-start" }}
            variant="ghost"
          >
            {open ? "Hide picks" : onEdit ? "Review & edit picks" : "Show picks"}
          </Action>
          {open &&
            // With an edit handler the disclosure IS the edit surface (V25b) — the same list,
            // with drop/add/note. Without one it stays read-only, so every other caller and the
            // member-facing views are unchanged.
            (onEdit ? (
              <ProposalEdit
                className="mt-2"
                lineup={lineup ?? []}
                acquisitions={acquisitionItems ?? []}
                alternates={alternates ?? []}
                episodeSelectionPreview={episodeSelectionPreview}
                value={edit}
                disabled={status === "approving"}
                // The title editor knows drops, adds and the note; a raised limit is kept across.
                onChange={(next) => onEdit(withAudience(next, edit?.audience))}
                showNote
                renderFeedback={renderFeedback}
              />
            ) : (
              <ul className="mt-2 flex flex-col gap-1.5">
                {lineup?.map((it) => (
                  <PickRow key={`lib-${it.name}`} item={it} kind="lineup" renderFeedback={renderFeedback} />
                ))}
                {acquisitionItems?.map((it) => (
                  <PickRow key={`acq-${it.name}`} item={it} kind="acquire" renderFeedback={renderFeedback} />
                ))}
              </ul>
            ))}
        </div>
      )}
    </Card>
  );
};

export { ApprovalQueueItem };

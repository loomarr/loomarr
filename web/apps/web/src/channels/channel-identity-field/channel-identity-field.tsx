import { Check, X } from "lucide-react";
import { useId, useState } from "react";
import { Button, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import type { ChannelIdentityFieldProps } from "./channel-identity-field.type";

// ChannelIdentityField — an inline-editable channel identity field (name / number) with
// explicit per-field confirmation. Unlike the old ghost inputs that committed silently on
// blur, this looks clearly editable, and CHANGING it reveals Save (✓) / Cancel (✕) so the
// operator confirms that one field before it commits. Enter saves, Escape cancels.
//
// `onSave` is async: it RESOLVES on success (the field adopts the new value + closes) or
// REJECTS on failure (e.g. a 409 duplicate number) — in which case the reason shows inline
// and the editor stays open. So a rejected renumber never silently swallows; the user fixes
// it in place. Validation blocks Save for a locally-invalid draft (empty name, number < 1)
// before any request goes out.
const ChannelIdentityField = ({
  label,
  value,
  type = "text",
  validate,
  onSave,
  variant = "title",
  className,
}: ChannelIdentityFieldProps) => {
  const id = useId();
  const committed = String(value);
  // The draft the input edits. Resets to `committed` on cancel / successful save. We track the
  // committed value we seeded from so a genuine server change (an SSE-driven refresh) adopts
  // without clobbering an in-progress edit — same render-time adjust pattern used elsewhere.
  const [draft, setDraft] = useState(committed);
  const [seeded, setSeeded] = useState(committed);
  const [saving, setSaving] = useState(false);
  const [serverError, setServerError] = useState<string>();
  if (committed !== seeded && draft === seeded && !saving) {
    // The server value changed and the user hasn't diverged — adopt it.
    setDraft(committed);
    setSeeded(committed);
  } else if (committed !== seeded) {
    // Server changed while the user was editing — track the new baseline for dirty compare,
    // but keep the user's draft.
    setSeeded(committed);
  }

  const validationError = validate?.(draft);
  const dirty = draft !== committed;
  const canSave = dirty && !validationError && !saving;

  const reset = () => {
    setDraft(committed);
    setServerError(undefined);
  };

  const save = async () => {
    if (!canSave) return;
    setSaving(true);
    setServerError(undefined);
    try {
      await onSave(type === "number" ? Number(draft) : draft.trim());
      // Success: the parent's mutation updated the source; `committed` will follow on the next
      // render. Nothing to reset here — dirty resolves once `value` reflects the save.
    } catch (e) {
      setServerError(e instanceof Error ? e.message : "Couldn't save — try a different value.");
    } finally {
      setSaving(false);
    }
  };

  const error = serverError ?? (dirty ? validationError : undefined);

  return (
    <div className={cn("flex flex-col gap-1", className)}>
      <div className="flex items-center gap-2">
        <Label htmlFor={id} className="sr-only">
          {label}
        </Label>
        <Input
          id={id}
          type={type}
          {...(type === "number" ? { min: 1 } : {})}
          value={draft}
          disabled={saving}
          aria-label={label}
          aria-invalid={error ? true : undefined}
          onChange={(e) => {
            setDraft(e.target.value);
            setServerError(undefined);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void save();
            } else if (e.key === "Escape") {
              e.preventDefault();
              reset();
            }
          }}
          className={cn(
            // Styled to read as intentionally editable (not the old transparent-border ghost).
            "border-input bg-static-800/40",
            variant === "title" && "h-10 max-w-xs font-semibold text-xl",
            variant === "compact" && "h-9 w-20 text-center font-mono tabular-nums",
            error && "border-onair-300 focus-visible:ring-onair-300",
          )}
        />
        {/* Save / Cancel appear ONLY when the field diverges from what's committed — the
            explicit per-field confirmation. */}
        {dirty && (
          <div className="flex shrink-0 items-center gap-1">
            <Button
              type="button"
              variant="suggest"
              size="icon"
              className="size-8"
              disabled={!canSave}
              aria-label={`Save ${label}`}
              onClick={() => void save()}
            >
              <Check className="size-4" aria-hidden />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-8"
              disabled={saving}
              aria-label={`Cancel ${label} edit`}
              onClick={reset}
            >
              <X className="size-4" aria-hidden />
            </Button>
          </div>
        )}
      </div>
      {error && <p className="text-onair-300 text-xs">{error}</p>}
    </div>
  );
};

export { ChannelIdentityField };

import { Action, Field, Stack, typography } from "@loomarr/design-system";
import { useId, useState } from "react";
import { useBelowSm } from "@/lib/use-phone-width";
import { cn } from "@/lib/utils";
import type { ChannelIdentityFieldProps } from "./channel-identity-field.type";

// ChannelIdentityField — an inline-editable channel identity field (name / number) with
// explicit per-field confirmation. Rather than a ghost input that commits silently on blur,
// this looks clearly editable, and CHANGING it reveals Save (✓) / Cancel (✕) so the
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

  // What a save sends, and so what the server stores: dirty compares this, not the raw draft, so a
  // padded draft stops being dirty once its trimmed value comes back as `value`.
  const normalised = type === "number" ? Number(draft) : draft.trim();
  const validationError = validate?.(draft);
  const dirty = String(normalised) !== committed;
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
      await onSave(normalised);
      // Success: the parent's mutation updated the source; `committed` follows on the next render.
      // The input shows the value that was saved.
      setDraft(String(normalised));
    } catch (e) {
      setServerError(e instanceof Error ? e.message : "Couldn't save. Try a different value.");
    } finally {
      setSaving(false);
    }
  };

  const error = serverError ?? (dirty ? validationError : undefined);

  // Below `sm` the name has its own row (the channel header, #1785), and a one-line input there
  // still cut a long name. So the title is a textarea that wraps to a second line instead: the
  // same field, one line of content. Enter saves rather than breaking the line, and a newline
  // that arrives some other way (a paste) becomes a space.
  const narrow = useBelowSm();
  const wraps = variant === "title" && narrow;

  return (
    <Stack className={className} gap="1">
      <Stack align="center" direction="row" gap="inline">
        {/* 448 px at most from `sm` up; below it the row is the name's alone (#1785). */}
        <div className={cn("min-w-0", variant === "title" ? "flex-1 sm:max-w-md" : "w-20")}>
          <Field
            accessibilityLabel={label}
            disabled={saving}
            id={id}
            invalid={Boolean(error)}
            keyboardType={type === "number" ? "number-pad" : "default"}
            multiline={wraps}
            numberOfLines={wraps ? 2 : undefined}
            onChangeText={(text) => {
              setDraft(text.replace(/[\r\n]+/g, " "));
              setServerError(undefined);
            }}
            onKeyPress={(event) => {
              const key = event.nativeEvent.key;
              if (key === "Enter") {
                event.preventDefault();
                void save();
              } else if (key === "Escape") {
                event.preventDefault();
                reset();
              }
            }}
            // The name reads as the channel's title, in the body face; the number as a tabular figure.
            style={
              variant === "title"
                ? { fontFamily: typography.family.body.web, fontSize: 20, fontWeight: "600" }
                : { textAlign: "center" }
            }
            value={draft}
          />
        </div>
        {/* Save / Cancel appear ONLY when the field diverges from what's committed — the
            explicit per-field confirmation. */}
        {dirty && (
          <Stack align="center" direction="row" flexShrink={0} gap="1">
            <Action
              disabled={!canSave}
              icon="success"
              layout="icon"
              onPress={() => void save()}
              tone="suggest"
            >
              {`Save ${label}`}
            </Action>
            <Action disabled={saving} icon="close" layout="icon" onPress={reset} variant="ghost">
              {`Cancel ${label} edit`}
            </Action>
          </Stack>
        )}
      </Stack>
      {error && (
        <p className="text-onair-300 text-xs" role="alert">
          {error}
        </p>
      )}
    </Stack>
  );
};

export { ChannelIdentityField };

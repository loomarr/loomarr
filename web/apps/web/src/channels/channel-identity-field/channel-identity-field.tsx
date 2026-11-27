import { Action, Field, Stack, Text, typography } from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { useId, useLayoutEffect, useState } from "react";
import { useAppForm } from "@/lib/app-form";
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
  // What a save sends, and so what the server stores: dirty compares this, not the raw draft, so a
  // padded draft stops being dirty once its trimmed value comes back as `value`.
  const normalise = (text: string) => (type === "number" ? Number(text) : text.trim());

  const [serverError, setServerError] = useState<string>();
  // The form's defaults are the stored value, and it takes a new one only while untouched.
  const form = useAppForm({
    defaultValues: { draft: committed },
    onSubmit: async ({ value, formApi }) => {
      const saved = normalise(value.draft);
      setServerError(undefined);
      try {
        await onSave(saved);
        // The input shows the value that was saved; the parent passes it as `value` once stored.
        formApi.setFieldValue("draft", String(saved));
      } catch (e) {
        setServerError(e instanceof Error ? e.message : "Couldn't save. Try a different value.");
      }
    },
  });
  const draft = useStore(form.store, (state) => state.values.draft);
  const saving = useStore(form.store, (state) => state.isSubmitting);
  // The field is untouched exactly when its draft is the stored value, whichever of the two moved
  // to meet the other. So a server change (an SSE-driven refresh) replaces a draft that has not
  // diverged, and leaves an edit in progress alone. A reset also ends a submit, so it waits for one.
  useLayoutEffect(() => {
    if (!saving && draft === committed && form.state.isTouched) form.reset();
  }, [committed, draft, form, saving]);

  const validationError = validate?.(draft);
  const dirty = String(normalise(draft)) !== committed;
  const canSave = dirty && !validationError && !saving;

  const reset = () => {
    form.reset();
    setServerError(undefined);
  };

  const save = () => {
    if (canSave) void form.handleSubmit();
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
          <form.Field name="draft">
            {(field) => (
              <Field
                accessibilityLabel={label}
                disabled={saving}
                id={id}
                invalid={Boolean(error)}
                keyboardType={type === "number" ? "number-pad" : "default"}
                multiline={wraps}
                numberOfLines={wraps ? 2 : undefined}
                onChangeText={(text) => {
                  field.handleChange(text.replace(/[\r\n]+/g, " "));
                  setServerError(undefined);
                }}
                onKeyPress={(event) => {
                  const key = event.nativeEvent.key;
                  if (key === "Enter") {
                    event.preventDefault();
                    save();
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
                value={field.state.value}
              />
            )}
          </form.Field>
        </div>
        {/* Save / Cancel appear ONLY when the field diverges from what's committed — the
            explicit per-field confirmation. */}
        {dirty && (
          <Stack align="center" direction="row" flexShrink={0} gap="1">
            <Action disabled={!canSave} icon="success" layout="icon" onPress={save} tone="suggest">
              {`Save ${label}`}
            </Action>
            <Action disabled={saving} icon="close" layout="icon" onPress={reset} variant="ghost">
              {`Cancel ${label} edit`}
            </Action>
          </Stack>
        )}
      </Stack>
      {error && (
        <div role="alert">
          <Text as="p" textRole="hint" tone="danger">
            {error}
          </Text>
        </div>
      )}
    </Stack>
  );
};

export { ChannelIdentityField };

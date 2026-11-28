import { Action, Chip, Field, Stack } from "@loomarr/design-system";
import type { IntentInputProps } from "./intent-input.type";

// IntentInput — the hero (§3). A natural-language channel intent wearing the
// `suggest` magenta focus ring (§2.1, the AI color), with template chips that kill
// the blank page (§6). Controlled, so a page wraps it in TanStack Form against
// intentSchema (packages/core) and the gallery drives every state deterministically.
// ⌘/Ctrl-Enter submits; the form owns validation so an invalid attempt can explain what to fix.
const IntentInput = ({
  value,
  onValueChange,
  onSubmit,
  templates = [],
  submitting = false,
  placeholder = "Describe a channel: 90s action movies, high energy, keep it PG-13",
}: IntentInputProps) => {
  const canSubmit = !submitting;
  const submit = () => {
    if (canSubmit) onSubmit?.();
  };
  return (
    <Stack gap="control">
      {/* The design system's text area: the one amber focus border, in place of the AI magenta ring. */}
      <Field
        accessibilityLabel="Channel intent"
        disabled={submitting}
        multiline
        numberOfLines={3}
        onChangeText={onValueChange}
        onKeyPress={(event) => {
          const { ctrlKey, key, metaKey } = event.nativeEvent as typeof event.nativeEvent & {
            ctrlKey?: boolean;
            metaKey?: boolean;
          };
          if (key === "Enter" && (metaKey || ctrlKey)) {
            event.preventDefault();
            submit();
          }
        }}
        placeholder={placeholder}
        value={value}
      />

      {templates.length > 0 && (
        <Stack direction="row" gap="inline" wrap>
          {templates.map((t) => (
            <Chip disabled={submitting} key={t.value} onPress={() => onValueChange(t.value)} shape="pill">
              {t.label}
            </Chip>
          ))}
        </Stack>
      )}

      <Stack direction="row" justify="end">
        <Action
          disabled={!canSubmit}
          icon={submitting ? "loading" : "suggest"}
          onPress={submit}
          tone="suggest"
        >
          {submitting ? "Suggesting…" : "Suggest a lineup"}
        </Action>
      </Stack>
    </Stack>
  );
};

export { IntentInput };

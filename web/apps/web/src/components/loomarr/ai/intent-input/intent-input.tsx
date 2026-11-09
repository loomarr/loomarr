import { Action, Chip } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
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
  className,
}: IntentInputProps) => {
  const canSubmit = !submitting;
  const submit = () => {
    if (canSubmit) onSubmit?.();
  };
  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <textarea
        value={value}
        onChange={(e) => onValueChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            submit();
          }
        }}
        placeholder={placeholder}
        rows={3}
        disabled={submitting}
        aria-label="Channel intent"
        className="border-input placeholder:text-muted-foreground focus-visible:border-suggest focus-visible:ring-suggest focus-visible:ring-offset-background w-full resize-none rounded-lg border bg-transparent px-4 py-3 text-base leading-relaxed shadow-sm transition-colors focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
      />

      {templates.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {templates.map((t) => (
            <Chip disabled={submitting} key={t.value} onPress={() => onValueChange(t.value)} shape="pill">
              {t.label}
            </Chip>
          ))}
        </div>
      )}

      <div className="flex justify-end">
        <Action
          disabled={!canSubmit}
          icon={submitting ? "loading" : "suggest"}
          onPress={submit}
          variant="suggest"
        >
          {submitting ? "Suggesting…" : "Suggest a lineup"}
        </Action>
      </div>
    </div>
  );
};

export { IntentInput };

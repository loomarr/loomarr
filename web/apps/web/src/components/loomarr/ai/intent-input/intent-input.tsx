import { Loader2, Sparkles } from "lucide-react";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import type { IntentInputProps } from "./intent-input.type";

// IntentInput — the hero (§3). A natural-language channel intent wearing the
// `suggest` magenta focus ring (§2.1, the AI color), with template chips that kill
// the blank page (§6). Controlled, so a page wraps it in TanStack Form against
// intentSchema (packages/core) and the gallery drives every state deterministically.
// ⌘/Ctrl-Enter submits; the button stays disabled until there's a describable intent.
const IntentInput = ({
  value,
  onValueChange,
  onSubmit,
  templates = [],
  submitting = false,
  placeholder = "Describe a channel: 90s action movies, high energy, keep it PG-13",
  className,
}: IntentInputProps) => {
  const canSubmit = value.trim().length >= 3 && !submitting;
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
        className="w-full resize-none rounded-lg border border-input bg-transparent px-4 py-3 text-base leading-relaxed shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:border-suggest focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-suggest focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50"
      />

      {templates.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {templates.map((t) => (
            <button
              key={t.value}
              type="button"
              disabled={submitting}
              onClick={() => onValueChange(t.value)}
              className="cursor-pointer rounded-full border border-border bg-static-800 px-3 py-1 text-static-400 text-xs transition-colors hover:border-suggest hover:text-suggest-300 disabled:opacity-50"
            >
              {t.label}
            </button>
          ))}
        </div>
      )}

      <div className="flex justify-end">
        <Button variant="suggest" onClick={submit} disabled={!canSubmit}>
          {submitting ? <Loader2 className="animate-spin" aria-hidden /> : <Sparkles aria-hidden />}
          {submitting ? "Suggesting…" : "Suggest a lineup"}
        </Button>
      </div>
    </div>
  );
};

export { IntentInput };

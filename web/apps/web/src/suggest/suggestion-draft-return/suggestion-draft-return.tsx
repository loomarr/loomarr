import { Action } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import { readSuggestionDraft } from "../suggestion-draft";

const SuggestionDraftReturn = () => {
  const draft = readSuggestionDraft();
  if (!draft) return null;

  return (
    <aside className="border-border bg-muted/40 flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3">
      <div>
        <p className="text-sm font-medium">Your channel draft is saved</p>
        <p className="text-muted-foreground text-sm">Finish setup, then return without retyping it.</p>
      </div>
      <Action accessibilityRole="link" render={<Link to="/requests/new" />} variant="outline">
        Return to channel
      </Action>
    </aside>
  );
};

export { SuggestionDraftReturn };

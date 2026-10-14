import { Link } from "@tanstack/react-router";
import { buttonVariants } from "@/components/ui/button";
import { readSuggestionDraft } from "../suggestion-draft";

const SuggestionDraftReturn = () => {
  const draft = readSuggestionDraft();
  if (!draft) return null;

  return (
    <aside className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-muted/40 px-4 py-3">
      <div>
        <p className="font-medium text-sm">Your channel draft is saved</p>
        <p className="text-muted-foreground text-sm">Finish setup, then return without retyping it.</p>
      </div>
      <Link to="/guide" className={buttonVariants({ variant: "outline", size: "sm" })}>
        Return to channel
      </Link>
    </aside>
  );
};

export { SuggestionDraftReturn };

import type { SearchResult } from "@loomarr/core";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { SearchCommand } from "@/components/loomarr";
import { usePaletteResults } from "../use-palette-results";
import type { CommandPaletteProps } from "./command-palette.type";

// CommandPalette — the ⌘K entry point (§12: "the single fast entry point").
//
// Mounted once in the app shell. CONTROLLED, because there are two ways in — the ⌘K
// shortcut and the shell's Search button — and they must drive one piece of state. They
// did not: the shell's button called a setter whose value was discarded
// (`const [, setCommandOpen] = useState(false)`), so clicking Search did nothing at all.
//
// SearchCommand stays presentational so the gallery renders it without a router or a
// query client; this owns the overlay and where a result takes you.
const CommandPalette = ({ open, onOpenChange }: CommandPaletteProps) => {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const { results, loading } = usePaletteResults(query);

  if (!open) return null;

  const setOpen = (next: boolean) => onOpenChange(next);

  const go = (result: SearchResult) => {
    setOpen(false);
    setQuery("");
    switch (result.scope) {
      case "channels":
        void navigate({ to: "/channels/$id", params: { id: result.id } });
        break;
      case "help":
        void navigate({ to: "/help", search: { page: result.id } });
        break;
      case "clips":
        // No per-clip route exists, so land on Filler pre-filtered to that clip rather
        // than inventing a detail page the backend has no endpoint for.
        void navigate({ to: "/filler" });
        break;
      default:
        // A title — library or TMDB — is something you'd act on by proposing a channel,
        // which is the Suggest workspace. Acquiring still routes through the approval
        // gate (§7); the palette never becomes a shortcut around it.
        void navigate({ to: "/suggest" });
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center p-4 pt-24">
      {/* The backdrop is a real <button>, not a div with a click handler. It is a dismiss
          CONTROL, so making it one satisfies the a11y rules honestly — no suppression —
          and gives screen-reader and keyboard users the same escape the pointer has.
          Rendering it as a sibling rather than a parent also removes the need to stop
          click propagation out of the dialog. */}
      <button
        type="button"
        aria-label="Close search"
        className="fixed inset-0 cursor-default bg-black/60"
        onClick={() => setOpen(false)}
      />
      <div role="dialog" aria-modal="true" aria-label="Search Loomarr" className="relative w-full max-w-xl">
        <SearchCommand
          query={query}
          onQueryChange={setQuery}
          results={results}
          loading={loading}
          onSelect={go}
        />
      </div>
    </div>
  );
};

export { CommandPalette };

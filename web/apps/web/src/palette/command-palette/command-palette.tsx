import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
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
//
// ⚠ **IT USED TO ONLY LOOK LIKE A MODAL.** This was a `fixed inset-0` div carrying
// `role="dialog" aria-modal="true"` with none of the behaviour either attribute promises: no
// portal, no focus trap, no focus restore, no scroll lock, no inert background. A screen reader
// was told the page behind was inert while Tab walked straight into it, and closing the palette
// dropped focus at the top of the document instead of returning it to whatever opened it. V50b
// makes the claim true rather than deleting it — unlike `RestartOverlay`, this one really is a
// dialog: it owns a text input and demands a response.
//
// ⚠ Built on the Dialog PRIMITIVE, not the app's `DialogContent` wrapper — same reason as
// `ClipPlayer`. The wrapper centres a padded `max-w-md` card and injects its own close button;
// the palette is a top-anchored `max-w-xl` surface with no chrome of its own.
//
// ⚠ ESCAPE IS NOW THE DIALOG'S. `useCommandShortcut` used to bind it at the window, because
// `SearchCommand` deliberately does not bind it by default — see that file's comment. The primitive
// owns Escape now, so the hook's Escape branch is gone; leaving both would have been harmless only
// by luck, and leaving neither would have made the key dead.
const CommandPalette = ({ open, onOpenChange }: CommandPaletteProps) => {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const { results, loading } = usePaletteResults(query);

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
        // which is the Guide's inline describe panel (§12: origination folded out of the
        // old /suggest page and into the channels surface). Acquiring still routes through
        // the approval gate (§7); the palette never becomes a shortcut around it.
        void navigate({ to: "/guide" });
    }
  };

  return (
    <DialogPrimitive.Root open={open} onOpenChange={setOpen}>
      <DialogPrimitive.Portal>
        {/* The backdrop no longer needs to be a <button>. It was one so that dismissing by
            pointer was a real control rather than a click handler on a div — an honest fix at the
            time. The primitive now dismisses on an outside press AND on Escape, for every input
            method, so a bespoke control would be a second way to do what the dialog already does. */}
        <DialogPrimitive.Backdrop className="fixed inset-0 z-50 bg-black/60" />
        <DialogPrimitive.Popup
          aria-label="Search Loomarr"
          className="fixed top-24 left-1/2 z-50 w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 focus:outline-none"
        >
          <SearchCommand
            query={query}
            onQueryChange={setQuery}
            results={results}
            loading={loading}
            onSelect={go}
          />
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
};

export { CommandPalette };

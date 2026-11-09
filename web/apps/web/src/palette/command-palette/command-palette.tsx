import type { SearchResult } from "@loomarr/core/contracts";
import { Dialog, DialogContent } from "@loomarr/design-system";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { SearchCommand } from "@/components/loomarr/shell/search-command";
import { usePhoneWidth } from "@/lib/use-phone-width";
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
// The design-system Dialog's `top` placement is this surface: near the top of the screen, no panel
// or corner close of its own. Below `md` it is `fill`, full-screen search (#2751): opened from the
// phone header's Search icon, it covers the page, the results take the height, and a Close button
// stands in for the backdrop and the Escape key a phone doesn't have.
//
// ⚠ ESCAPE IS NOW THE DIALOG'S. `useCommandShortcut` used to bind it at the window, because
// `SearchCommand` deliberately does not bind it by default — see that file's comment. The primitive
// owns Escape now, so the hook's Escape branch is gone; leaving both would have been harmless only
// by luck, and leaving neither would have made the key dead.
const CommandPalette = ({ open, onOpenChange }: CommandPaletteProps) => {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const { results, loading } = usePaletteResults(query);
  const phoneWidth = usePhoneWidth();

  const setOpen = (next: boolean) => onOpenChange(next);

  const go = (result: SearchResult) => {
    setOpen(false);
    setQuery("");
    switch (result.scope) {
      case "diagnostics":
        void navigate({ to: "/settings/system/diagnostics", search: { view: "logs" } });
        break;
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
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent accessibilityLabel="Search Loomarr" placement={phoneWidth ? "fill" : "top"}>
        <SearchCommand
          query={query}
          onQueryChange={setQuery}
          results={results}
          loading={loading}
          onSelect={go}
          fill={phoneWidth}
          onClose={phoneWidth ? () => setOpen(false) : undefined}
        />
      </DialogContent>
    </Dialog>
  );
};

export { CommandPalette };

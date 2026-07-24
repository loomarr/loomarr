import { useEffect } from "react";

// The suffix every browser-tab title carries, so a bare tab reads "Loomarr" and a page
// reads "Channels | Loomarr" — the station name always last (the broadcast-console
// identity, §1). Kept here (not per call site) so the format is set once.
const SUFFIX = "Loomarr";

// useDocumentTitle sets the browser-tab title to `${title} | Loomarr` for the lifetime of
// the calling screen, restoring the previous title on unmount so navigating away never
// leaves a stale tab. A falsy title (e.g. a channel name still loading) sets just the bare
// suffix, so the tab reads "Loomarr" until the data arrives rather than "undefined | Loomarr".
//
// A hook (not TanStack `head:`) because one title is DYNAMIC — the channel-detail tab is the
// channel's name — and a hook handles static and data-derived titles uniformly, with no root
// HeadContent wiring. Web-only (touches document), so it lives in lib.
const useDocumentTitle = (title?: string | null) => {
  useEffect(() => {
    const previous = document.title;
    document.title = title ? `${title} | ${SUFFIX}` : SUFFIX;
    return () => {
      document.title = previous;
    };
  }, [title]);
};

export { useDocumentTitle };

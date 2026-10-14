import type { Page } from "@playwright/test";

// One piece of content a person cannot see or reach: its box leaves an ancestor that clips.
interface ClippedContent {
  element: string;
  text: string;
  side: "left" | "right" | "top" | "bottom";
  px: number;
  clippedBy: string;
}

// Lists the interactive elements and text boxes on the page whose box extends more than a pixel
// past an `overflow: hidden | clip` ancestor (#1785). A document-width check misses this: the page
// at 390 px had no horizontal scroll while a card clipped its buttons and a list clipped its rows.
//
// A scroll container is not a clip: what it hides is reachable by scrolling, so once the walk up
// passes a scroller on an axis, that axis stops counting. Text an ellipsis truncates is shown as
// truncated, so it passes too. Visually hidden content (a box of a pixel
// or less, aria-hidden, inert) is skipped.
//
// Runs in the browser, so it is self-contained: no imports, no closures over this module.
const findClippedContent = (page: Page): Promise<ClippedContent[]> =>
  page.evaluate(() => {
    const clips = (value: string) => value === "hidden" || value === "clip";
    const scrolls = (value: string) => value === "auto" || value === "scroll";
    const interactive =
      "a[href],button,input,select,textarea,[role=button],[role=link],[role=tab],[tabindex]:not([tabindex='-1'])";
    const candidates = new Set<Element>(document.querySelectorAll(interactive));
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      if (node.textContent?.trim() && node.parentElement) candidates.add(node.parentElement);
    }

    const found: ClippedContent[] = [];
    for (const element of candidates) {
      const style = getComputedStyle(element);
      if (style.visibility === "hidden" || style.display === "none") continue;
      if (element.closest("[aria-hidden=true],[inert]")) continue;
      const box = element.getBoundingClientRect();
      if (box.width <= 1 || box.height <= 1) continue;

      let checkX = true;
      let checkY = true;
      for (
        let ancestor = element.parentElement;
        ancestor && ancestor !== document.documentElement && (checkX || checkY);
        ancestor = ancestor.parentElement
      ) {
        const s = getComputedStyle(ancestor);
        const clipX = checkX && clips(s.overflowX);
        const clipY = checkY && clips(s.overflowY);
        if (scrolls(s.overflowX)) checkX = false;
        if (scrolls(s.overflowY)) checkY = false;
        if (!clipX && !clipY) continue;
        // A deliberate truncation: the ellipsis tells the reader there is more.
        if (s.textOverflow === "ellipsis") break;
        const frame = ancestor.getBoundingClientRect();
        if (frame.width <= 1 || frame.height <= 1) break; // a visually hidden wrapper
        const overhang = {
          left: clipX ? frame.left - box.left : 0,
          right: clipX ? box.right - frame.right : 0,
          top: clipY ? frame.top - box.top : 0,
          bottom: clipY ? box.bottom - frame.bottom : 0,
        };
        const [side, px] = (Object.entries(overhang) as [ClippedContent["side"], number][]).reduce((a, b) =>
          b[1] > a[1] ? b : a,
        );
        if (px > 1) {
          found.push({
            element: element.tagName.toLowerCase(),
            text: (element.getAttribute("aria-label") ?? element.textContent ?? "").trim().slice(0, 60),
            side,
            px: Math.round(px),
            clippedBy: `${ancestor.tagName.toLowerCase()} ${String(ancestor.className).slice(0, 60)}`,
          });
          break;
        }
      }
    }
    return found;
  });

export { type ClippedContent, findClippedContent };

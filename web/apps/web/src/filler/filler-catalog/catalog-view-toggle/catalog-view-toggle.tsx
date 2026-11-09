import { Action, Tooltip } from "@loomarr/design-system";
import { useRef } from "react";
import { cn } from "@/lib/utils";

// The catalog's two renderings (V35b, the mock's `catViews`). Grid is the default because filler is
// picked by LOOKING at it — a wall of frames is the point; the list is for scanning a large catalog
// and bulk-selecting, where a thumbnail per row is noise.
//
// Both views open the shared clip panel; bulk selection remains independent.
const VIEWS = [
  { id: "grid", label: "Grid", icon: "grid", title: "Cards with thumbnails and clip details" },
  { id: "list", label: "List", icon: "list", title: "A dense row per clip, for scanning and selecting" },
] as const;

type CatalogView = (typeof VIEWS)[number]["id"];

interface CatalogViewToggleProps {
  /** The route's `view` search value; anything but a known view reads as none selected. */
  view: string;
  /** A phone shows icons only and keeps the toggle beside the Filters button. */
  phone: boolean;
  onChange: (view: CatalogView) => void;
}

// Grid ⇄ list (V35b). ⚠ A radiogroup, not two toggle buttons: these are two states of ONE setting,
// and a pair of independent buttons announces neither which is active nor that they are
// alternatives. On a wide screen it sits at the far end of the toolbar, as the mock draws it; on a
// phone it follows the Filters button. Arrow keys, Home and End move the choice, roving tabindex.
// The chosen view is the DS's neutral-pressed secondary, not the amber primary: amber marks focus.
const CatalogViewToggle = ({ view, phone, onChange }: CatalogViewToggleProps) => {
  const refs = useRef<Array<{ focus: () => void } | null>>([]);
  const onKeyDown = (event: { key: string; preventDefault: () => void }, index: number) => {
    let next = index;
    if (event.key === "ArrowRight" || event.key === "ArrowDown") next = (index + 1) % VIEWS.length;
    else if (event.key === "ArrowLeft" || event.key === "ArrowUp")
      next = (index - 1 + VIEWS.length) % VIEWS.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = VIEWS.length - 1;
    else return;

    event.preventDefault();
    refs.current[next]?.focus();
    const nextView = VIEWS[next];
    if (nextView) onChange(nextView.id);
  };

  return (
    <div
      className={cn("ml-auto flex gap-1", !phone && "order-last")}
      role="radiogroup"
      aria-label="Clip view"
    >
      {VIEWS.map((v, index) => (
        <Tooltip content={v.title} key={v.id}>
          <Action
            accessibilityLabel={v.label}
            accessibilityRole="radio"
            aria-checked={view === v.id}
            icon={v.icon}
            layout={phone ? "icon" : "inline"}
            onPress={() => onChange(v.id)}
            ref={(node) => {
              refs.current[index] = node;
            }}
            selected={view === v.id}
            tabIndex={view === v.id ? 0 : -1}
            tone="secondary"
            {...({
              onKeyDown: (event: { key: string; preventDefault: () => void }) => onKeyDown(event, index),
            } as object)}
          >
            {v.label}
          </Action>
        </Tooltip>
      ))}
    </div>
  );
};

export type { CatalogView };
export { CatalogViewToggle };

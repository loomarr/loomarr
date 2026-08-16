import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { useHoldControls } from "../internal/hold-controls-context";
import type { TrackSelectMenuProps } from "./track-select-menu.type";

// TrackSelectMenu — an icon-triggered single-select in the player's control bar (§9.1 V47). Audio
// and Subtitles are the same shape (an icon opens a list; one option is current), so this one
// component serves both. It composes the core DropdownMenu; the player just places two of these
// beside fullscreen.
//
// ⚠ Audio/subtitles are ADMIN-SCOPED + CHANNEL-WIDE (one encoder per channel fans to every viewer,
// so a per-viewer track would fork the encode). A member sees the current track — the menu opens and
// shows the check — but every option is disabled, with a one-line note saying why.
//
// While open it HOLDS the player's controls shown (useHoldControls) — otherwise the auto-hide would
// pull the bar (and this trigger) away from under the open menu.
const TrackSelectMenu = ({ icon: Icon, label, options, value, onChange, readOnly }: TrackSelectMenuProps) => {
  const { hold } = useHoldControls();
  return (
    <DropdownMenu onOpenChange={hold}>
      {/* Icon ONLY (like the fullscreen button) — the current track shows as a check IN the menu.
          Same hover-grow as the other controls. */}
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            className={cn(
              "size-8 shrink-0 rounded-full text-static-300",
              "transition-transform hover:scale-110 hover:bg-transparent hover:text-static-0 active:scale-95 motion-reduce:transition-none",
            )}
            aria-label={label}
          />
        }
      >
        <Icon aria-hidden />
      </DropdownMenuTrigger>
      {/* Opens UPWARD (side="top") — the bar sits at the bottom of the frame, so a menu below would be
        clipped off-screen. */}
      <DropdownMenuContent side="top" align="end" className="min-w-44">
        {/* The label NAMES the group rather than floating above it — Base UI has no standalone
            menu label, and the group is what a screen reader announces the rows as belonging to. */}
        <DropdownMenuGroup>
          <DropdownMenuLabel>{label}</DropdownMenuLabel>
          {options.map((o) => (
            <DropdownMenuCheckboxItem
              key={o.value}
              checked={o.value === value}
              // Read-only for a member: the row shows its state but cannot be toggled.
              disabled={readOnly}
              onCheckedChange={() => onChange(o.value)}
            >
              {o.label}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuGroup>
        {readOnly && (
          <>
            <DropdownMenuSeparator />
            <p className="px-2 py-1.5 text-2xs text-muted-foreground leading-snug">
              Set for the whole channel — an admin changes it for everyone.
            </p>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export { TrackSelectMenu };

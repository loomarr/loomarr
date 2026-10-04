import { Badge, BottomSheet, Icon, icons, Surface, TabBar, Text } from "@loomarr/design-system";
import { useLocation, useNavigate } from "@tanstack/react-router";
import type { ComponentRef } from "react";
import { useEffect, useId, useRef, useState } from "react";
import { Pressable } from "react-native";
import { channelNavHighlight } from "@/channels/channel-nav-highlight";
import {
  type PhoneDestination,
  type PhoneDestinationKey,
  phoneBarOverflow,
  phoneBarPrimary,
  phoneDestinationMatches,
} from "../phone-bottom-bar-destinations";
import type { PhoneBottomBarProps } from "./phone-bottom-bar.type";

// A value that matches no real destination, so `TabBar`'s `selected` prop can legitimately light
// no tab at all — e.g. the viewer is on an overflow page (Settings, Help, …) or on a route the
// bar doesn't know about (/account). `TabBar` only compares equality; it never requires `selected`
// to be one of `items`. "more" is the trailing disclosure button's own value — also never lit.
type BarValue = PhoneDestinationKey | "more" | "none";

const navigateToDestination = (navigate: ReturnType<typeof useNavigate>, destination: PhoneDestination) => {
  if (destination.to === "/channels/$id/watch") {
    void navigate({ to: destination.to, params: destination.params });
  } else {
    void navigate({ to: destination.to });
  }
};

const SheetRow = ({ destination, onSelect }: { destination: PhoneDestination; onSelect: () => void }) => (
  <Pressable
    accessibilityLabel={destination.label}
    accessibilityRole="button"
    onPress={onSelect}
    style={{ alignItems: "center", flexDirection: "row", gap: 12, minHeight: 48, width: "100%" }}
  >
    <Icon decorative glyph={icons[destination.icon]} size="control" tone="secondary" />
    <Text density="touch" flex={1} textRole="body">
      {destination.label}
    </Text>
    {destination.badgeCount ? (
      <Badge density="touch" tone="danger">
        {destination.badgeCount}
      </Badge>
    ) : null}
  </Pressable>
);

/**
 * Web's phone-width "More": the admin/member destinations that don't fit the bar (#1785, Alt A,
 * approved 2026-10-03). A docked raised surface (`BottomSheet` from #1776, no grabber — this is a
 * plain list, not drag-to-dismiss content), behind a full-screen backdrop that owns the dialog
 * semantics `BottomSheet` itself doesn't provide on web: focus moves in on open, `Escape`/backdrop/
 * close dismiss, a Tab loop keeps focus inside while open, and focus returns to the More button.
 *
 * The backdrop and the dialog's own viewport anchor are the only two plain (unstyled) DOM nodes
 * here: `position: fixed` is a web-only concept `@loomarr/design-system`'s View props don't carry
 * (RN only knows "absolute"/"relative"), so there is nothing in the design system to place a
 * full-screen overlay with. Everything inside them is a design-system view.
 */
const MoreSheet = ({
  destinations,
  onClose,
  onSelect,
}: {
  destinations: PhoneDestination[];
  onClose: () => void;
  onSelect: (destination: PhoneDestination) => void;
}) => {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<ComponentRef<typeof Pressable>>(null);

  useEffect(() => {
    closeRef.current?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key !== "Tab" || !dialogRef.current) return;
      const focusable = dialogRef.current.querySelectorAll<HTMLElement>(
        'button, [href], [tabindex]:not([tabindex="-1"])',
      );
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (!first || !last) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  return (
    <>
      <button
        aria-hidden="true"
        data-testid="phone-bottom-bar-backdrop"
        onClick={onClose}
        style={{
          background: "rgba(0, 0, 0, 0.5)",
          border: "none",
          inset: 0,
          padding: 0,
          position: "fixed",
          zIndex: 50,
        }}
        tabIndex={-1}
        type="button"
      />
      <div
        aria-labelledby={titleId}
        aria-modal="true"
        ref={dialogRef}
        role="dialog"
        style={{ bottom: 0, left: 0, position: "fixed", right: 0, zIndex: 50 }}
      >
        <BottomSheet accessibilityLabel="More destinations" handle={false}>
          <Surface
            alignItems="center"
            backgroundColor="$transparent"
            borderWidth={0}
            flexDirection="row"
            justifyContent="space-between"
          >
            <Text density="touch" id={titleId} textRole="label">
              More
            </Text>
            <Pressable
              accessibilityLabel="Close"
              accessibilityRole="button"
              onPress={onClose}
              ref={closeRef}
              style={{ alignItems: "center", height: 32, justifyContent: "center", width: 32 }}
            >
              <Icon decorative glyph={icons.close} size="control" tone="secondary" />
            </Pressable>
          </Surface>
          <Surface backgroundColor="$transparent" borderWidth={0}>
            {destinations.map((destination) => (
              <SheetRow
                destination={destination}
                key={destination.key}
                onSelect={() => onSelect(destination)}
              />
            ))}
          </Surface>
        </BottomSheet>
      </div>
    </>
  );
};

/**
 * The web shell's phone-width (< `md`) footer nav, replacing the 56 px icon rail (#1785, #1659
 * Shell section, Alt A approved 2026-10-03). Home/Watch/Guide/Requests are fixed bar slots — Watch
 * absent until `watchChannelId` exists (X2) — and everything else for the role sits behind More.
 */
const PhoneBottomBar = ({ badges, isAdmin, watchChannelId }: PhoneBottomBarProps) => {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [sheetOpen, setSheetOpen] = useState(false);

  const primary = phoneBarPrimary(watchChannelId, badges);
  const overflow = phoneBarOverflow(isAdmin, badges);
  // Decision X2 (critique row 5), shared with the desktop rail: a channel's Watch tab highlights
  // Watch, its management tabs (Info/Programming/Filler/Danger) highlight Guide instead — the URL
  // stays under `/channels/$id/...` for both, so the plain per-destination path match below can't
  // tell them apart on its own.
  const highlight = channelNavHighlight(pathname);
  const all = [...primary, ...overflow];
  const active =
    (highlight && all.find((destination) => destination.key === highlight)) ||
    all.find((destination) => phoneDestinationMatches(pathname, destination));
  const selected: BarValue = primary.some((destination) => destination.key === active?.key)
    ? (active?.key as PhoneDestinationKey)
    : "none";
  const overflowHasBadge = overflow.some((destination) => (destination.badgeCount ?? 0) > 0);

  // `TabBar` owns the More button's DOM node; there's nothing else to hold a ref to it against,
  // so closing (Escape, backdrop, the sheet's own × — never a destination pick, which navigates
  // away on purpose) finds it back by the `aria-label`/`role` pair only this one button has.
  const closeSheet = () => {
    setSheetOpen(false);
    document.querySelector<HTMLElement>('[aria-label="More"][role="button"]')?.focus();
  };

  const items = [
    ...primary.map((destination) => ({
      badge: (destination.badgeCount ?? 0) > 0,
      icon: destination.icon,
      label: destination.label,
      value: destination.key as BarValue,
    })),
    ...(overflow.length > 0
      ? [
          {
            ariaExpanded: sheetOpen,
            badge: overflowHasBadge,
            icon: "more" as const,
            kind: "disclosure" as const,
            label: "More",
            value: "more" as BarValue,
          },
        ]
      : []),
  ];

  return (
    <>
      <TabBar<BarValue>
        accessibilityLabel="Primary navigation"
        idiom="ios"
        items={items}
        onSelect={(value) => {
          if (value === "more") {
            setSheetOpen(true);
            return;
          }
          const destination = primary.find((item) => item.key === value);
          if (destination) navigateToDestination(navigate, destination);
        }}
        selected={selected}
      />
      {sheetOpen ? (
        <MoreSheet
          destinations={overflow}
          onClose={closeSheet}
          onSelect={(destination) => {
            setSheetOpen(false);
            navigateToDestination(navigate, destination);
          }}
        />
      ) : null}
    </>
  );
};

export { PhoneBottomBar };

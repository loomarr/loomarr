import {
  Badge,
  BottomSheet,
  Dialog,
  DialogContent,
  Icon,
  icons,
  Surface,
  TabBar,
  Text,
} from "@loomarr/design-system";
import { useLocation, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { Pressable } from "react-native";
import { channelNavHighlight } from "@/channels/channel-nav-highlight";
import {
  type PhoneDestination,
  type PhoneDestinationKey,
  phoneBarOverflow,
  phoneBarPrimary,
  phoneDestinationMatches,
} from "./destinations";
import type { PhoneBottomBarProps } from "./phone-bottom-bar.type";

// A value that matches no real destination, so `TabBar`'s `selected` prop can legitimately light
// no tab at all — e.g. the viewer is on an overflow page (Settings, Help, …) or on a route the
// bar doesn't know about (/account). `TabBar` only compares equality; it never requires `selected`
// to be one of `items`. "more" is the trailing disclosure button's own value — also never lit.
type BarValue = PhoneDestinationKey | "more" | "none";

const navigateToDestination = (navigate: ReturnType<typeof useNavigate>, destination: PhoneDestination) => {
  if (destination.to === "/watch/$id") {
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
 * plain list, not drag-to-dismiss content), held by the design system's bottom-placed `Dialog`,
 * which owns the dialog semantics `BottomSheet` itself doesn't provide on web: focus moves in on
 * open, `Escape`/backdrop/close dismiss, a Tab loop keeps focus inside while open, and focus returns
 * to the More button.
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
  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent accessibilityLabel="More" placement="bottom">
        <BottomSheet accessibilityLabel="More destinations" handle={false}>
          <Surface
            alignItems="center"
            backgroundColor="transparent"
            borderWidth={0}
            flexDirection="row"
            justifyContent="space-between"
          >
            <Text density="touch" textRole="label">
              More
            </Text>
            <Pressable
              accessibilityLabel="Close"
              accessibilityRole="button"
              onPress={onClose}
              style={{ alignItems: "center", height: 32, justifyContent: "center", width: 32 }}
            >
              <Icon decorative glyph={icons.close} size="control" tone="secondary" />
            </Pressable>
          </Surface>
          <Surface backgroundColor="transparent" borderWidth={0}>
            {destinations.map((destination) => (
              <SheetRow
                destination={destination}
                key={destination.key}
                onSelect={() => onSelect(destination)}
              />
            ))}
          </Surface>
        </BottomSheet>
      </DialogContent>
    </Dialog>
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
  // Decision X2 (critique row 5), shared with the desktop rail: a channel's `/watch/$id` player
  // highlights Watch, its management tabs (Info/Programming/Filler/Danger) highlight Guide instead —
  // those URLs stay under `/channels/$id/...`, so the plain per-destination path match below can't
  // tell them apart from the channel on its own.
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

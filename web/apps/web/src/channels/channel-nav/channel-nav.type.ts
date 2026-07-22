type ChannelNavSection = {
  id: string;
  label: string;
};

type ChannelNavProps = {
  // The sections to list, in order — the same registry that drives the content column.
  sections: ChannelNavSection[];
  // The id of the section currently in view (from the scroll-spy), highlighted in the menu.
  activeId: string;
  // Clicking a menu item — the page expands + scrolls to that section.
  onSelect: (id: string) => void;
  className?: string;
};

export type { ChannelNavProps, ChannelNavSection };

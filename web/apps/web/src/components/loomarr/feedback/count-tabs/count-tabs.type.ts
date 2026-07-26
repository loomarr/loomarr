type CountTab = {
  id: string;
  label: string;
  // Shown as a pill beside the label. The count and the list it labels must come from the same
  // source — a tab that says 3 above a list of 2 is worse than no count at all.
  count: number;
};

type CountTabsProps = {
  tabs: CountTab[];
  activeId: string;
  onSelect: (id: string) => void;
  // Names the tablist for screen readers ("Queue sections"). Required rather than defaulted:
  // a generic label on a page with two tab bars tells a screen-reader user nothing.
  label: string;
  className?: string;
};

export type { CountTab, CountTabsProps };

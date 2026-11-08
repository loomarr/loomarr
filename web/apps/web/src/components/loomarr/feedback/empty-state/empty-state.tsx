import { StatePanel } from "@loomarr/ui";
import type { EmptyStateProps } from "./empty-state.type";

// EmptyState — mandatory for every list (§6): the design system's StatePanel (static colour bars,
// a sentence-case heading, one plain sentence, exactly ONE next action) so every empty list reads
// like the 404, the crash boundary and the player cover.
const EmptyState = ({ title, description, action, className }: EmptyStateProps) => (
  <div className={className}>
    <StatePanel
      action={action ? { label: action.label, onPress: () => action.onClick?.() } : undefined}
      description={description}
      kind="empty"
      title={title}
    />
  </div>
);

export { EmptyState };

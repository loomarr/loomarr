/**
 * Where one component sits across its parent's cross axis. It is the one layout a primitive takes
 * from its caller beside Text's flex-item props; space around it belongs to the parent `Stack`.
 */
type ItemLayoutProps = {
  alignSelf?: "auto" | "center" | "flex-end" | "flex-start" | "stretch";
};

export type { ItemLayoutProps };

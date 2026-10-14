import type { PreviewAnchorProps, PreviewGroupProps } from "./preview.type";

// Native hosts keep their touch/remote detail adapters.
const PreviewGroup = ({ children }: PreviewGroupProps) => <>{children}</>;
const PreviewAnchor = ({ children }: PreviewAnchorProps) => <>{children}</>;

export { PreviewAnchor, PreviewGroup };

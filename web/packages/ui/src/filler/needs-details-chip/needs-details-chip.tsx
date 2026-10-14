import { Badge } from "@loomarr/design-system";

// The one exception a READY filler clip can carry (#1817): a commercial missing a match tag. A
// caution chip because it is a warning the operator can act on (#1909). The caller renders it only
// from the server's `ClipDTO.needsDetails`, the same predicate as the Library's "Untagged only"
// filter, never from kind + tagged, so the chip and the filter cannot disagree.
const NeedsDetailsChip = () => (
  <Badge shape="square" tone="caution">
    Needs details
  </Badge>
);

export { NeedsDetailsChip };

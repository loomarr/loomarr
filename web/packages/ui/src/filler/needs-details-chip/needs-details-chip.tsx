import { StatusLabel } from "@loomarr/design-system";

// The one exception a READY filler clip can carry (#1817): a commercial missing a match tag. A
// warning dot and word because it is a warning the operator can act on (#1909). The caller renders it only
// from the server's `ClipDTO.needsDetails`, the same predicate as the Library's "Untagged only"
// filter, never from kind + tagged, so the label and the filter cannot disagree.
const NeedsDetailsChip = () => <StatusLabel tone="warn">Needs details</StatusLabel>;

export { NeedsDetailsChip };

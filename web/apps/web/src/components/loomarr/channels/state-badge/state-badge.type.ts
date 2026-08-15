import type { TitleDTOState } from "@loomarr/api";

// The 5 title provisioning states come 1:1 from the generated TitleDTOState (§12); `drift`
// is a FE-only badge state (a title whose library item changed underneath it) with no BE
// enum value, so it's added explicitly rather than hand-mirroring the whole union.
type ProvisioningState = TitleDTOState | "drift";

interface StateBadgeProps {
  state: ProvisioningState;
  className?: string;
}

export type { ProvisioningState, StateBadgeProps };

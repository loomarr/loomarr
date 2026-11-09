import { Action, Surface } from "@loomarr/design-system";
import type { PhoneHeaderProps } from "./phone-header.type";

// The phone's top row (#2751, decided 2026-11-08): a Search icon, top right on every page, opening
// the one search the desktop rail's Search button opens, full screen at this width. Not in the
// bottom bar or the More sheet. Lazy in AppShell for the same bundle reason as PhoneBottomBar.
const PhoneHeader = ({ onOpenSearch }: PhoneHeaderProps) => (
  <Surface
    alignItems="center"
    borderRadius={0}
    // ⚠ Tamagui applies props in order: borderWidth must come before the one side it keeps.
    borderWidth={0}
    borderBottomWidth={1}
    flexDirection="row"
    flexShrink={0}
    justifyContent="flex-end"
    paddingHorizontal={8}
    role="banner"
  >
    <Action density="touch" icon="search" layout="icon" onPress={onOpenSearch} variant="ghost">
      Search
    </Action>
  </Surface>
);

export { PhoneHeader };

import type { ReactNode } from "react";
import { type StyleProp, View, type ViewStyle } from "react-native";

// A dialog that sits in the page rather than over it: the player's menu, sheets and panel. It is
// `role="dialog"` with a name and nothing else. It has no scrim, no portal, no focus trap, no scroll
// lock and no Escape of its own, so the picture beside it keeps playing and Tab walks straight out.
// The caller draws its surface (the player's chrome, which differs between the menu, the sheets and
// the panel) and places it; what the primitive owns is the dialog contract.
//
// That contract is what the player reads: while a layer is open, the arrow keys are the layer's to
// move through its rows, so the player's own key handler leaves an open dialog's keys alone. One
// primitive means every layer says so the same way. A blocking dialog is `Dialog`; a panel docked
// to an edge with its own header is `DockedPanel`.
//
// One component serves the web and native: React Native for Web renders the View as a `div` with
// the role and name, and a native build maps the role onto the platform's dialog trait.

interface NonModalDialogProps {
  children: ReactNode;
  /** The dialog's name, read by a screen reader and found by the player's key handler. */
  label: string;
  /** The caller's surface and placement. */
  style?: StyleProp<ViewStyle>;
  testID?: string;
}

const NonModalDialog = ({ children, label, style, testID }: NonModalDialogProps) => (
  <View aria-label={label} role="dialog" style={style} testID={testID}>
    {children}
  </View>
);

export type { NonModalDialogProps };
export { NonModalDialog };

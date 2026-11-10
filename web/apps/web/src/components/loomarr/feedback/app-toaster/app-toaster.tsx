import { Toaster } from "sonner";
import { usePhoneWidth } from "@/lib/use-phone-width";

// Wide layouts: bottom-centre, lifted clear of a page's persistent bottom actions.
const ABOVE_BOTTOM_ACTIONS = 84;
// Phone width: under the shell's 49px header row (its Search button stays pressable), plus a gap.
const BELOW_PHONE_HEADER = 57;

// AppToaster — the app's one toast region. At phone width the bottom band holds the tab bar and a
// settings page's sticky save bar, so a toast drawn there lies over Save changes right after
// "Couldn't save your changes" (#3439). Toasts go to the top at that width, where nothing a
// failed action needs is pinned. Sonner switches to `mobileOffset` at 600px and the phone layout
// runs to 48rem, so both offsets carry the same top inset.
const AppToaster = () => {
  const phone = usePhoneWidth();
  return (
    <Toaster
      theme="dark"
      position={phone ? "top-center" : "bottom-center"}
      offset={phone ? { top: BELOW_PHONE_HEADER } : { bottom: ABOVE_BOTTOM_ACTIONS }}
      mobileOffset={{ top: BELOW_PHONE_HEADER }}
      richColors
    />
  );
};

export { AppToaster };

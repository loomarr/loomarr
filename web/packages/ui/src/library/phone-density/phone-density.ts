import { useWindowWidth } from "@loomarr/design-system";

// A window narrower than a tablet is tapped, so its controls take the design system's touch targets.
const usePhoneDensity = () => (useWindowWidth() >= 768 ? "pointer" : "touch");

export { usePhoneDensity };

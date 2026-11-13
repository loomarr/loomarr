import { BrandWordmark, Stack, Text } from "@loomarr/design-system";
import { ColorBars } from "../color-bars";
import type { BrandLockupProps } from "./brand-lockup.type";

// BrandLockup — the LOOMARR mark: the test-card strip over/beside the wordmark (§1). Two
// forms: `hero` (centered, big, tagline) for the login + wizard, and `compact`
// (horizontal, small) for the app sidebar. The wordmark is Geist, all-caps, wide-tracked;
// the tagline is mono, the same "nostalgia lives in microcopy" register as the rest (§1).
const BrandLockup = ({ variant = "hero", tagline = true, className }: BrandLockupProps) => {
  if (variant === "compact") {
    return (
      <Stack align="center" className={className} direction="row" gap="inline">
        <ColorBars size="sm" />
        <BrandWordmark size="small" />
      </Stack>
    );
  }
  return (
    <Stack align="center" className={className} gap="control">
      <ColorBars size="lg" />
      <BrandWordmark size="large" />
      {tagline && (
        <Text as="span" textRole="mono" tone="secondary">
          always something on
        </Text>
      )}
    </Stack>
  );
};

export { BrandLockup };

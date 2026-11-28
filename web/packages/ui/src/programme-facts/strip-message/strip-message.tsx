import { Stack, Surface, Text, ThemeScope } from "@loomarr/design-system";

interface StripMessageProps {
  /** The words to say; empty draws nothing, and the live region stays to announce the next one. */
  message: string | undefined;
}

/** A message raised over the picture or the Guide, on ink: when a later programme starts, say. */
const StripMessage = ({ message }: StripMessageProps) => (
  <Stack align="center" aria-live="polite" flexShrink={0} position="relative" role="status" zIndex={0}>
    {message ? (
      <ThemeScope theme="dark">
        <Surface
          backgroundColor="surfaceCanvas"
          borderRadius={8}
          borderWidth={0}
          paddingHorizontal={14}
          paddingVertical={8}
        >
          <Text textAlign="center" textRole="label">
            {message}
          </Text>
        </Surface>
      </ThemeScope>
    ) : null}
  </Stack>
);

export type { StripMessageProps };
export { StripMessage };

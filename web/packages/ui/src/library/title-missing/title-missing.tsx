import { Action, Text } from "@loomarr/design-system";
import { Line, PAGE_GUTTER, Stack } from "../library-poster";
import { useTitleLayout } from "../title-parts";

// A title the Library doesn't hold for this viewer: an unknown id, or one hidden from a member.
// There is nothing to retry, so the page offers only the way back.
const LibraryTitleMissing = ({ onBack }: { onBack: () => void }) => {
  const { wide, density } = useTitleLayout();
  return (
    <Stack gap="control" padding={wide ? PAGE_GUTTER.wide : PAGE_GUTTER.phone}>
      <Text as="h1" textRole="headline">
        This title isn't in the library yet
      </Text>
      <Line>
        <Action density={density} icon="back" onPress={onBack} variant="outline">
          Back to Library
        </Action>
      </Line>
    </Stack>
  );
};

export { LibraryTitleMissing };

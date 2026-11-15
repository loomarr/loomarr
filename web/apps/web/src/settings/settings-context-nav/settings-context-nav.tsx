import { Action, Stack, Text } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";

interface SettingsContextNavProps {
  section?: string;
}

// Settings pages deliberately avoid a permanent sibling menu. This small wayfinding row gives
// every leaf a predictable route back to the task home without making all settings look equally
// important again.
const SettingsContextNav = ({ section }: SettingsContextNavProps) => (
  <Stack
    align="center"
    borderBottomWidth={1}
    borderColor="$borderDecorative"
    className="px-4 sm:px-6"
    direction="row"
    flexShrink={0}
    gap="control"
    justify="between"
  >
    <Stack aria-label="Settings location" minWidth={0} render="nav">
      <Stack align="center" direction="row" gap="inline" minWidth={0} render="ol">
        <li>
          <Action accessibilityRole="link" icon="goBack" render={<Link to="/settings" />} variant="ghost">
            Settings
          </Action>
        </li>
        {section ? (
          <>
            <Text as="li" aria-hidden textRole="compact" tone="secondary">
              /
            </Text>
            <Text as="li" className="truncate" textRole="compact" tone="primary">
              {section}
            </Text>
          </>
        ) : null}
      </Stack>
    </Stack>
    {/* The short "Find" is the narrow label; both are the one link, named in full either way. */}
    <Stack flexShrink={0} hidden={{ base: true, sm: false }}>
      <Action
        accessibilityLabel="Find another setting"
        accessibilityRole="link"
        icon="search"
        render={<Link to="/settings" />}
        variant="ghost"
      >
        Find another setting
      </Action>
    </Stack>
    <Stack flexShrink={0} hidden={{ base: false, sm: true }}>
      <Action
        accessibilityLabel="Find another setting"
        accessibilityRole="link"
        icon="search"
        render={<Link to="/settings" />}
        variant="ghost"
      >
        Find
      </Action>
    </Stack>
  </Stack>
);

export { SettingsContextNav };

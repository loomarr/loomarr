import { Action, Icon, icons, Stack, Text } from "@loomarr/design-system";
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
    minHeight={44}
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
    <Link
      to="/settings"
      aria-label="Find another setting"
      className="text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex shrink-0 items-center gap-1.5 rounded-sm focus-visible:ring-2 focus-visible:outline-none"
    >
      <Icon decorative glyph={icons.search} size="compact" tone="inherit" />
      {/* The label takes the link's own colour (and its hover), so the role sets only the type. */}
      <Stack hidden={{ base: true, sm: false }} render="span">
        <Text as="span" style={{ color: "inherit" }} textRole="compact">
          Find another setting
        </Text>
      </Stack>
      <Stack hidden={{ base: false, sm: true }} render="span">
        <Text as="span" style={{ color: "inherit" }} textRole="compact">
          Find
        </Text>
      </Stack>
    </Link>
  </Stack>
);

export { SettingsContextNav };

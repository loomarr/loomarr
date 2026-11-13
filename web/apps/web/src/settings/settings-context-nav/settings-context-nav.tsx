import { Action, Icon, icons, Stack } from "@loomarr/design-system";
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
    className="border-border min-h-11 border-b px-4 sm:px-6"
    direction="row"
    flexShrink={0}
    gap="control"
    justify="between"
  >
    <nav aria-label="Settings location" className="min-w-0">
      <Stack
        align="center"
        className="min-w-0 text-sm"
        direction="row"
        flexShrink={1}
        gap="inline"
        render="ol"
      >
        <li>
          <Action accessibilityRole="link" icon="goBack" render={<Link to="/settings" />} variant="ghost">
            Settings
          </Action>
        </li>
        {section ? (
          <>
            <li className="text-muted-foreground" aria-hidden>
              /
            </li>
            <li className="text-foreground truncate">{section}</li>
          </>
        ) : null}
      </Stack>
    </nav>
    <Link
      to="/settings"
      aria-label="Find another setting"
      className="text-muted-foreground hover:text-foreground focus-visible:ring-ring inline-flex shrink-0 items-center gap-1.5 rounded-sm text-sm focus-visible:ring-2 focus-visible:outline-none"
    >
      <Icon decorative glyph={icons.search} size="compact" tone="inherit" />
      <span className="hidden sm:inline">Find another setting</span>
      <span className="sm:hidden">Find</span>
    </Link>
  </Stack>
);

export { SettingsContextNav };

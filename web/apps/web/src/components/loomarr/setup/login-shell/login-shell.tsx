import { Card, Stack } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import { BrandLockup, TvStatic } from "../../shell";
import type { LoginShellProps } from "./login-shell.type";

// LoginShell — the presentational frame for the sign-in page (§11, §13): the test-card
// hero over the CRT static, with the auth form in a centered card. Extracted from the
// login route (which owns the router + mutation) so the assembled page — brand, static,
// and card together — is a gallery-testable unit; the route only supplies `children`.
// Same split as WizardShell / SettingsFields: presentation here, data-binding at the route.
const LoginShell = ({ children, className }: LoginShellProps) => (
  <Stack
    align="center"
    className={cn("bg-background relative p-6", className)}
    gap="8"
    justify="center"
    // A prop, not `min-h-screen`: the View base rule's `min-height: 0` is unlayered and beats the
    // utility, which collapsed the page to its content and lifted the card off centre.
    minHeight="100vh"
    render="main"
  >
    {/* Static sits behind; content siblings get an explicit stacking context above it, since
        a position:absolute layer paints OVER position:static content regardless of DOM order. */}
    <TvStatic className="z-0" />
    <BrandLockup variant="hero" className="relative z-10" />
    {/* `gap` spaces the demo site's sign-in card (#2268) from the form; one child is unaffected. */}
    <Card gap={20} maxWidth={384} padding={32} position="relative" width="100%" zIndex={10}>
      {children}
    </Card>
  </Stack>
);

export { LoginShell };

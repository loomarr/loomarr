import { Card, CardContent } from "@/components/ui";
import { cn } from "@/lib";
import { BrandLockup } from "../brand-lockup";
import { TvStatic } from "../tv-static";
import type { LoginShellProps } from "./login-shell.type";

// LoginShell — the presentational frame for the sign-in page (§11, §13): the test-card
// hero over the CRT static, with the auth form in a centered card. Extracted from the
// login route (which owns the router + mutation) so the assembled page — brand, static,
// and card together — is a gallery-testable unit; the route only supplies `children`.
// Same split as WizardShell / SettingsFields: presentation here, data-binding at the route.
const LoginShell = ({ children, className }: LoginShellProps) => (
  <main
    className={cn(
      "relative flex min-h-screen flex-col items-center justify-center gap-8 bg-background p-6",
      className,
    )}
  >
    <TvStatic />
    <BrandLockup variant="hero" />
    <Card className="w-full max-w-sm">
      <CardContent className="p-8">{children}</CardContent>
    </Card>
  </main>
);

export { LoginShell };

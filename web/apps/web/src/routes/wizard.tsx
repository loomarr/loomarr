import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

// First-run onboarding (§13). Public — its bootstrap step creates the owning admin
// before any session exists (§11). The 7-step stepper lands in 13.3b/c.
const WizardScreen = () => (
  <Placeholder title="Setup wizard" hint="First-run onboarding lands here in 13.3." />
);

const Route = createFileRoute("/wizard")({
  component: WizardScreen,
});

export { Route };

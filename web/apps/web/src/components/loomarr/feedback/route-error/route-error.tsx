import { toProblem } from "@loomarr/api/mutator";
import { Surface } from "@loomarr/design-system";
import { StatePanel } from "@loomarr/ui";
import { type ErrorComponentProps, useRouter } from "@tanstack/react-router";

// RouteError — the router's default error boundary, so a route whose guard or loader fails shows
// the problem in the app's words with a retry, not TanStack's bare fallback. It is the router
// default rather than the root route's errorComponent because a root boundary would replace the
// whole root layout, restart overlay included; as the default, the failing route's own boundary
// catches it and everything above it stays mounted.
const RouteError = ({ error, reset }: ErrorComponentProps) => {
  const router = useRouter();
  const problem = toProblem(error);
  return (
    <Surface backgroundColor="transparent" borderWidth={0} padding="section">
      <StatePanel
        action={{
          label: "Try again",
          onPress: () => {
            reset();
            void router.invalidate();
          },
        }}
        description={problem.detail ?? problem.title ?? "Loomarr couldn't answer. Try again in a moment."}
        kind="error"
        title="Couldn't load this page"
      />
    </Surface>
  );
};

export { RouteError };

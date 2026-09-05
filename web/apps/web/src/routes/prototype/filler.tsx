import { createFileRoute, useNavigate, useSearch } from "@tanstack/react-router";
import { FillerIncomingPrototype, type FillerPrototypeVariant } from "./-filler-incoming-prototype";

const ids: FillerPrototypeVariant[] = ["review", "flow", "console"];

// PROTOTYPE — intentionally public on this throwaway branch so it can be reviewed without a
// configured backend or authenticated development session. Production builds render nothing.
const FillerPrototypeScreen = () => {
  const navigate = useNavigate();
  const search = useSearch({ strict: false }) as { variant?: string };
  if (!import.meta.env.DEV) return null;
  const variant = ids.includes(search.variant as FillerPrototypeVariant)
    ? (search.variant as FillerPrototypeVariant)
    : "flow";

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-border border-b bg-card px-6 py-4">
        <div className="mx-auto flex max-w-[1500px] flex-wrap items-center gap-x-8 gap-y-3">
          <span className="font-semibold tracking-tight">Loomarr</span>
          <span className="text-muted-foreground text-sm">Filler</span>
          <nav className="flex gap-5 text-muted-foreground text-sm">
            <span className="border-signal border-b-2 pb-1 text-foreground">Overview</span>
            <span>Sources</span>
            <span>Incoming</span>
            <span>Library</span>
            <span>Manage</span>
          </nav>
          <span className="ml-auto text-muted-foreground text-xs">Prototype · no writes</span>
        </div>
      </header>
      <main className="mx-auto max-w-[1500px] p-6">
        <FillerIncomingPrototype
          variant={variant}
          onVariantChange={(next) =>
            void navigate({ to: "/prototype/filler", search: { variant: next }, replace: true })
          }
        />
      </main>
    </div>
  );
};

const Route = createFileRoute("/prototype/filler")({ component: FillerPrototypeScreen });

export { Route };

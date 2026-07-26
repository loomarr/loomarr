import type { PlaceholderProps } from "./placeholder.type";

// Placeholder page content for the 13.1 skeleton — the real screens land in 13.3/13.4.
// Every list gets an EmptyState with exactly one next action (§6); this stands in for
// that pattern until the surfaces are built. Rendered by the stub routes under _authed.
const Placeholder = ({ title, hint }: PlaceholderProps) => (
  <div className="flex h-full flex-col">
    <header className="border-border border-b px-6 py-4">
      <h1 className="font-semibold text-xl">{title}</h1>
    </header>
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="max-w-sm text-center">
        <p className="font-mono text-sm text-static-400 uppercase tracking-wide">Dead air</p>
        <p className="mt-2 text-muted-foreground">{hint}</p>
      </div>
    </div>
  </div>
);

export { Placeholder };

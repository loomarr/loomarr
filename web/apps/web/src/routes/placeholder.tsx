// Placeholder page for the 13.1 skeleton — the real screens land in 13.3/13.4.
// Every list gets an EmptyState with exactly one next action (§6); this stands in
// for that pattern until the surfaces are built.
export function Placeholder({ title, hint }: { title: string; hint: string }) {
  return (
    <div className="flex h-full flex-col">
      <header className="border-b border-border px-6 py-4">
        <h1 className="text-xl font-semibold">{title}</h1>
      </header>
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="max-w-sm text-center">
          <p className="font-mono text-sm uppercase tracking-wide text-static-500">Dead air</p>
          <p className="mt-2 text-muted-foreground">{hint}</p>
        </div>
      </div>
    </div>
  );
}

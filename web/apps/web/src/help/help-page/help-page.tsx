import { helpApi } from "@loomarr/api";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { ErrorState } from "@/components/loomarr";
import { Card, Input, Label } from "@/components/ui";
import { DocView } from "../doc-view";
import { docMatches } from "../search-docs";

// HelpPage — the embedded documentation (§13). Pages ship inside the binary, so this
// works air-gapped, and search is client-side (§7.2: household-scale markdown needs no
// backend index).
//
// The open page lives in the URL (?page=troubleshooting#anchor) because the API already
// emits deep-links of exactly that shape on every failed setup check — a red check must
// be able to land the operator on the right section, not just the right page.
const HelpPage = () => {
  const navigate = useNavigate();
  const { page, section } = useSearch({ from: "/_authed/help" });
  const [q, setQ] = useState("");

  const list = helpApi.useListDocs();
  const docs = list.data?.status === 200 ? (list.data.data.docs ?? []) : [];

  // Default to the first page rather than an empty pane: Help with nothing open reads
  // as broken, and troubleshooting sorts first by slug in practice.
  const active = page ?? docs[0]?.slug;

  const doc = helpApi.useGetDoc(active ?? "", { query: { enabled: Boolean(active) } });
  const markdown = doc.data?.status === 200 ? doc.data.data.markdown : "";

  // Scroll to the deep-linked section once its page has rendered (§13: a red check lands
  // on the exact heading, not just the page). Runs when the section OR the rendered
  // markdown changes, so switching pages re-targets and a slow doc load still lands.
  useEffect(() => {
    if (!section || !markdown) return;
    document.getElementById(section)?.scrollIntoView({ block: "start" });
  }, [section, markdown]);

  // Searching filters the NAV, not the prose: the corpus is a handful of pages, so
  // narrowing which page to open is the useful operation — not excerpting matches.
  const visible = q ? docs.filter((d) => docMatches(d.title, markdown, d.slug === active, q)) : docs;

  if (list.error) return <ErrorState error={list.error} onRetry={() => list.refetch()} />;

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="font-semibold text-xl">Help</h1>
        <p className="mt-1 text-muted-foreground text-sm">Ships with Loomarr — no internet needed.</p>
      </div>

      <div className="flex flex-col gap-6 md:flex-row">
        <nav aria-label="Help pages" className="md:w-56 md:shrink-0">
          <Card className="flex flex-col gap-3 p-3">
            <div>
              <Label htmlFor="help-search">Search</Label>
              <Input
                id="help-search"
                value={q}
                placeholder="Filter pages"
                onChange={(e) => setQ(e.target.value)}
              />
            </div>
            <ul className="flex flex-col gap-1">
              {visible.map((d) => (
                <li key={d.slug}>
                  <button
                    type="button"
                    aria-current={d.slug === active ? "page" : undefined}
                    onClick={() => navigate({ to: "/help", search: { page: d.slug } })}
                    className={
                      d.slug === active
                        ? "w-full cursor-pointer rounded-md bg-static-800 px-2 py-1.5 text-left text-sm"
                        : "w-full cursor-pointer rounded-md px-2 py-1.5 text-left text-muted-foreground text-sm hover:bg-static-900"
                    }
                  >
                    {d.title}
                  </button>
                </li>
              ))}
              {visible.length === 0 && (
                <li className="px-2 py-1.5 text-muted-foreground text-sm">No pages match.</li>
              )}
            </ul>
          </Card>
        </nav>

        <div className="min-w-0 flex-1">
          {doc.error ? (
            <ErrorState error={doc.error} onRetry={() => doc.refetch()} />
          ) : doc.isLoading ? (
            <p className="text-muted-foreground text-sm">Loading…</p>
          ) : (
            <DocView
              markdown={markdown}
              onNavigate={(nextPage, nextSection) =>
                navigate({ to: "/help", search: { page: nextPage, section: nextSection } })
              }
            />
          )}
        </div>
      </div>
    </div>
  );
};

export { HelpPage };

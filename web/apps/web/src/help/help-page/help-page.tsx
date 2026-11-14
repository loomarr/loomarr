import * as helpApi from "@loomarr/api/endpoints/help";
import { unwrap } from "@loomarr/api/unwrap";
import { Card, Stack, Text } from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { useDocumentTitle } from "@/lib/use-document-title";
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
  useDocumentTitle("Help");
  const navigate = useNavigate();
  const { page, section } = useSearch({ from: "/_authed/help" });
  const [q, setQ] = useState("");

  const list = helpApi.useListDocs();
  const docs = unwrap(list.data, (b) => b.docs) ?? [];

  // Default to the first page rather than an empty pane: Help with nothing open reads
  // as broken, and troubleshooting sorts first by slug in practice.
  const active = page ?? docs[0]?.slug;

  const doc = helpApi.useGetDoc(active ?? "", section ? { section } : undefined, {
    query: { enabled: Boolean(active) },
  });
  const served = unwrap(doc.data, (b) => b);
  const markdown = served?.markdown ?? "";

  // A link from before the help restructure (#1572) can name a page or section that moved,
  // sometimes to another page. The server answers with where that content lives now, and the
  // URL follows it so the right page is open and the heading scroll below lands (#1736).
  useEffect(() => {
    if (!page || !served) return;
    if (served.slug === page && (served.section === section || !section)) return;
    void navigate({
      to: "/help",
      search: { page: served.slug, ...(served.section ? { section: served.section } : {}) },
      replace: true,
    });
  }, [navigate, page, section, served]);

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
    <Stack height="100%" minHeight={0}>
      <PageHeader title="Help" description="Ships with Loomarr. No internet needed." />

      <Stack
        className="overflow-auto"
        direction={{ base: "column", md: "row" }}
        flexBasis="0%"
        flexGrow={1}
        flexShrink={1}
        gap="6"
        minHeight={0}
        padding={24}
      >
        <nav aria-label="Help pages" className="md:w-56 md:shrink-0">
          <Card gap={12} padding={12}>
            <SearchField label="Search" onChangeText={setQ} placeholder="Filter pages" value={q} />
            <Stack gap="1" render="ul">
              {visible.map((d) => (
                <li key={d.slug}>
                  {/* A link, not a button: each page is a place with its own URL, so it opens in a new
                      tab and reads as navigation to assistive tech (#2751). */}
                  <Link
                    to="/help"
                    search={{ page: d.slug }}
                    aria-current={d.slug === active ? "page" : undefined}
                    className={
                      d.slug === active
                        ? "bg-static-800 block w-full rounded-md px-2 py-1.5 text-left text-sm"
                        : "text-muted-foreground hover:bg-static-900 block w-full rounded-md px-2 py-1.5 text-left text-sm"
                    }
                  >
                    {d.title}
                  </Link>
                </li>
              ))}
              {visible.length === 0 && (
                <Text as="li" className="px-2 py-1.5" textRole="compact" tone="secondary">
                  No pages match.
                </Text>
              )}
            </Stack>
          </Card>
        </nav>

        <div className="min-w-0 flex-1">
          {doc.error ? (
            <ErrorState error={doc.error} onRetry={() => doc.refetch()} />
          ) : doc.isLoading ? (
            <Text as="p" textRole="compact" tone="secondary">
              Loading…
            </Text>
          ) : (
            <DocView
              markdown={markdown}
              onNavigate={(nextPage, nextSection) =>
                navigate({ to: "/help", search: { page: nextPage, section: nextSection } })
              }
            />
          )}
        </div>
      </Stack>
    </Stack>
  );
};

export { HelpPage };

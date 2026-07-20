interface DocViewProps {
  markdown: string;
  className?: string;
  // Called when the reader clicks an INTERNAL doc link (a relative "concepts" or
  // "member-guide#section" — not an http URL or a same-page "#anchor"). The parent routes
  // it into the Help center. Absent ⇒ internal links render as plain relative anchors,
  // which is only correct when DocView isn't inside the routed Help page.
  onNavigate?: (page: string, section?: string) => void;
}

export type { DocViewProps };

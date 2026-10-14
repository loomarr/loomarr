// docMatches decides whether a help page survives the nav filter.
//
// Only the OPEN page's body is available to search — the list endpoint returns slugs and
// titles, and fetching every page to grep it would trade an offline nicety for N requests.
// So a page matches on its title, and additionally on its body when it happens to be the
// one already loaded. That is honest about the limit rather than pretending to full-text
// search: §7.2 keeps Help search client-side precisely because the corpus is small.
const docMatches = (title: string, markdown: string, isActive: boolean, query: string): boolean => {
  const q = query.trim().toLowerCase();
  if (q === "") return true;
  if (title.toLowerCase().includes(q)) return true;
  return isActive && markdown.toLowerCase().includes(q);
};

export { docMatches };

// The backend owns which BCP 47 codes are supported. Language names are interface copy, so each
// client derives them from CLDR in its active locale rather than receiving English from the API.
const languageName = (code: string, locale?: string): string => {
  try {
    const name = new Intl.DisplayNames(locale ? [locale] : undefined, {
      type: "language",
      fallback: "none",
    }).of(code);
    if (name) return name;
  } catch {
    // An unknown/malformed detector code must remain visible rather than breaking the review UI.
  }
  return code.toLocaleUpperCase();
};

export { languageName };

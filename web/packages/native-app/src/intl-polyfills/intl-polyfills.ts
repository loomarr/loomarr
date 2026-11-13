// Hermes ships Intl.Collator, DateTimeFormat and NumberFormat but not Intl.DisplayNames, so
// @loomarr/core's languageLabel fell back to the bare code: the Subtitles and Audio menus read
// "EN" on the TV and phone where the web reads "English". These fill exactly that gap with the
// same CLDR data the browser uses, so the label logic stays one function on every client.
//
// Each `polyfill.js` installs only when the runtime lacks the API (or lacks the locale), and the
// order is the dependency order: DisplayNames resolves its locale through Locale, which
// canonicalises through getCanonicalLocales. English names only: the app's UI is English.
import "@formatjs/intl-getcanonicallocales/polyfill.js";
import "@formatjs/intl-locale/polyfill.js";
import "@formatjs/intl-displaynames/polyfill.js";
import "@formatjs/intl-displaynames/locale-data/en.js";

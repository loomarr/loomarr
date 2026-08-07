package taxonomy

// The shipped default forest (§10 V45a) — what an install starts with before any operator edit. It
// covers the composites-design taxonomy and the original flat-12 `category` values, arranged on axes.
//
// ⚠ **Slugs are a stable contract.** They are what the tagger emits, what `clip_tags` references, and
// what a curation rule names. Renaming one is a breaking change — add a RetiredAlias instead so old
// tags still resolve. Adding a taxon is safe; removing one an operator may have tagged clips under is
// not (the reindex would orphan those rows), so the seed only grows.
//
// ⚠ The original flat categories appear here as PRODUCT LEAVES or FORMAT taxa so a clip tagged under
// the old model still resolves: `toys`, `cars`, `tech`, `fast_food`, `cereal`, `candy` are product
// leaves; `psa`, `movie_trailer`, `ident`, `bumper` are formats; `games` a product; `general` is
// deliberately NOT a taxon (it meant "untagged", which is now the empty tag set).

// SeedForest returns the default taxonomy. Build it into a *Forest with New.
func SeedForest() []Taxon {
	return []Taxon{
		// ── product axis ────────────────────────────────────────────────────────────────────────
		// Roots
		{Slug: "drinks", Label: "Drinks", Axis: AxisProduct, Synonyms: []string{"beverage", "beverages"}},
		{Slug: "food", Label: "Food", Axis: AxisProduct},
		{Slug: "household", Label: "Household", Axis: AxisProduct, Synonyms: []string{"home", "cleaning"}},
		{Slug: "tech", Label: "Technology", Axis: AxisProduct, Synonyms: []string{"technology", "electronics"}},
		{Slug: "cars", Label: "Automotive", Axis: AxisProduct, Synonyms: []string{"car", "auto", "automotive", "trucks", "vehicles"}},
		{Slug: "apparel", Label: "Apparel", Axis: AxisProduct, Synonyms: []string{"clothing", "fashion", "shoes"}},
		{Slug: "retail", Label: "Retail", Axis: AxisProduct, Synonyms: []string{"store", "shopping"}},
		{Slug: "toys", Label: "Toys", Axis: AxisProduct, Synonyms: []string{"toy"}},
		{Slug: "games", Label: "Games", Axis: AxisProduct, Synonyms: []string{"videogame", "video-game", "gaming"}},
		{Slug: "travel", Label: "Travel", Axis: AxisProduct, Synonyms: []string{"airline", "hotel", "tourism"}},
		{Slug: "local", Label: "Local business", Axis: AxisProduct, Synonyms: []string{"regional", "local-business"}},
		{Slug: "movies", Label: "Movies & entertainment", Axis: AxisProduct, Synonyms: []string{"film", "entertainment"}},
		// drinks children
		{Slug: "soda", Label: "Soda", Parent: "drinks", Axis: AxisProduct, Synonyms: []string{"soft-drink", "pop", "cola"}},
		{Slug: "juice", Label: "Juice", Parent: "drinks", Axis: AxisProduct},
		{Slug: "water", Label: "Water", Parent: "drinks", Axis: AxisProduct, Synonyms: []string{"bottled-water"}},
		{Slug: "coffee", Label: "Coffee & tea", Parent: "drinks", Axis: AxisProduct, Synonyms: []string{"tea"}},
		{Slug: "alcohol", Label: "Alcohol", Parent: "drinks", Axis: AxisProduct, Synonyms: []string{"alcoholic"}},
		{Slug: "beer", Label: "Beer", Parent: "alcohol", Axis: AxisProduct, Synonyms: []string{"brew", "lager", "ale"}},
		{Slug: "spirits", Label: "Spirits", Parent: "alcohol", Axis: AxisProduct, Synonyms: []string{"liquor", "whiskey", "vodka"}},
		// food children
		{Slug: "fast_food", Label: "Fast food", Parent: "food", Axis: AxisProduct, Synonyms: []string{"fastfood", "burger", "pizza", "quick-service"}},
		{Slug: "cereal", Label: "Cereal", Parent: "food", Axis: AxisProduct, Synonyms: []string{"breakfast-cereal"}},
		{Slug: "candy", Label: "Candy", Parent: "food", Axis: AxisProduct, Synonyms: []string{"chocolate", "sweets", "confectionery"}},
		{Slug: "snacks", Label: "Snacks", Parent: "food", Axis: AxisProduct, Synonyms: []string{"chips", "snack"}},
		{Slug: "frozen", Label: "Frozen food", Parent: "food", Axis: AxisProduct, Synonyms: []string{"frozen-food", "ice-cream"}},
		// tech children
		{Slug: "telecom", Label: "Telecom", Parent: "tech", Axis: AxisProduct, Synonyms: []string{"phone", "wireless", "long-distance", "telephone"}},
		{Slug: "appliances", Label: "Appliances", Parent: "tech", Axis: AxisProduct, Synonyms: []string{"appliance"}},
		{Slug: "computers", Label: "Computers & software", Parent: "tech", Axis: AxisProduct, Synonyms: []string{"software", "pc"}},
		// cars children
		{Slug: "dealer", Label: "Car dealer", Parent: "cars", Axis: AxisProduct, Synonyms: []string{"dealership"}},
		// local children
		{Slug: "restaurant-local", Label: "Local restaurant", Parent: "local", Axis: AxisProduct},
		{Slug: "service-local", Label: "Local service", Parent: "local", Axis: AxisProduct},
		// movies children
		{Slug: "movie_trailer", Label: "Movie trailer", Parent: "movies", Axis: AxisProduct, Synonyms: []string{"trailer", "coming-soon"}},

		// ── format axis (what the clip IS, not what it sells) ────────────────────────────────────
		{Slug: "commercial", Label: "Commercial", Axis: AxisFormat, Synonyms: []string{"ad", "advert", "spot"}},
		{Slug: "psa", Label: "PSA", Axis: AxisFormat, Synonyms: []string{"public-service", "public-service-announcement"}},
		{Slug: "promo", Label: "Network promo", Axis: AxisFormat, Synonyms: []string{"promotion", "tune-in"}},
		{Slug: "ident", Label: "Station ident", Axis: AxisFormat, Synonyms: []string{"station-id", "identification"}},
		{Slug: "bumper", Label: "Bumper", Axis: AxisFormat, Synonyms: []string{"we-ll-be-right-back"}},
		{Slug: "interstitial", Label: "Interstitial", Axis: AxisFormat},

		// ── seasonal axis (reuses the §10 curation holiday IDs) ──────────────────────────────────
		{Slug: "christmas", Label: "Christmas", Axis: AxisSeasonal, Synonyms: []string{"xmas", "holiday-christmas"}},
		{Slug: "halloween", Label: "Halloween", Axis: AxisSeasonal},
		{Slug: "thanksgiving", Label: "Thanksgiving", Axis: AxisSeasonal},
		{Slug: "back-to-school", Label: "Back to school", Axis: AxisSeasonal, Synonyms: []string{"back2school"}},
		{Slug: "memorial-day", Label: "Memorial Day", Axis: AxisSeasonal},
		{Slug: "holiday", Label: "Holiday (general)", Axis: AxisSeasonal},

		// ── audience-cue axis (HINTS, separate from the audience verdict) ─────────────────────────
		{Slug: "kids-cue", Label: "Kids-oriented cue", Axis: AxisAudienceCue, Synonyms: []string{"kids-toys", "for-kids"}},
		{Slug: "family-cue", Label: "Family cue", Axis: AxisAudienceCue, Synonyms: []string{"family-values"}},
		{Slug: "late-night-cue", Label: "Late-night cue", Axis: AxisAudienceCue, Synonyms: []string{"adult-cue", "late-night-adult"}},
	}
}

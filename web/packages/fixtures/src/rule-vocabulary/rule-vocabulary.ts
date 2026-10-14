import type { Vocabulary } from "@loomarr/api/models/vocabulary";

// A static rule authoring vocabulary for STORIES + UNIT TESTS — the runtime rules editor gets
// the real one from GET /v1/programming/vocabulary (ChannelProgramming passes it as a prop),
// so this is deterministic test scaffolding, not a runtime source (the byte-for-byte runtime
// mirror of internal/schedule/presets.go is gone). Values match presets.go so a story's
// picker renders exactly as it does live.
const ruleVocabularyFixture: Vocabulary = {
  when: [
    { token: "weekend", label: "Weekend", shortLabel: "Weekend", predicate: { weekend: true }, priority: 20 },
    { token: "weekday", label: "Weekday", shortLabel: "Weekday", predicate: { weekday: true }, priority: 20 },
    {
      token: "mornings",
      label: "Mornings (6–10)",
      shortLabel: "Mornings",
      predicate: { hourFrom: 6, hourTo: 10 },
      priority: 30,
    },
    {
      token: "daytime",
      label: "Daytime (10–17)",
      shortLabel: "Daytime",
      predicate: { hourFrom: 10, hourTo: 17 },
      priority: 30,
    },
    {
      token: "primetime",
      label: "Primetime (20–23)",
      shortLabel: "Primetime",
      predicate: { hourFrom: 20, hourTo: 23 },
      priority: 40,
    },
    {
      token: "late-night",
      label: "Late night (23–2)",
      shortLabel: "Late night",
      predicate: { hourFrom: 23, hourTo: 2 },
      priority: 40,
    },
    {
      token: "overnight",
      label: "Overnight (2–6)",
      shortLabel: "Overnight",
      predicate: { hourFrom: 2, hourTo: 6 },
      priority: 40,
    },
    {
      token: "holiday:christmas",
      label: "Christmas",
      shortLabel: "Christmas",
      predicate: { holiday: "christmas" },
      priority: 60,
    },
    {
      token: "holiday:halloween",
      label: "Halloween",
      shortLabel: "Halloween",
      predicate: { holiday: "halloween" },
      priority: 60,
    },
    {
      token: "holiday:thanksgiving",
      label: "Thanksgiving",
      shortLabel: "Thanksgiving",
      predicate: { holiday: "thanksgiving" },
      priority: 60,
    },
    {
      token: "holiday:newyear",
      label: "New Year",
      shortLabel: "New Year",
      predicate: { holiday: "newyear" },
      priority: 60,
    },
    {
      token: "holiday:valentines",
      label: "Valentine's Day",
      shortLabel: "Valentine's Day",
      predicate: { holiday: "valentines" },
      priority: 60,
    },
  ],
  what: [
    { token: "all", label: "Anything (no extra narrowing)" },
    {
      token: "kids",
      label: "Kids-safe genres",
      scope: { genres: { include: ["Animation", "Family", "Kids"] } },
    },
    {
      token: "family",
      label: "Family genres",
      scope: { genres: { include: ["Family", "Animation", "Adventure", "Comedy"] } },
    },
    { token: "holiday-matched", label: "Holiday-matched titles" },
  ],
  how: [
    {
      token: "syndication",
      label: "Syndication (the deck order)",
      shortLabel: "Syndication",
      ordering: { ordering: "syndication" },
      windowFull: false,
    },
    {
      token: "shuffle",
      label: "Shuffle",
      shortLabel: "Shuffle",
      ordering: { ordering: "shuffle" },
      windowFull: false,
    },
    {
      token: "marathon",
      label: "Marathon (binge one show, no breaks)",
      shortLabel: "Marathon",
      ordering: { ordering: "sequential", noBreaks: true, separation: { blockMax: -1 } },
      windowFull: true,
    },
    {
      token: "feature",
      label: "Feature (in order)",
      shortLabel: "Feature",
      ordering: { ordering: "sequential" },
      windowFull: false,
    },
  ],
};

export { ruleVocabularyFixture };

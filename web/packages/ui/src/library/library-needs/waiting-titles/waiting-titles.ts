// How many titles wait: Needs you draws one card each, so a title in two lists counts once.
const waitingTitles = (...lists: readonly (readonly { id: string }[])[]): number =>
  new Set(lists.flatMap((list) => list.map((title) => title.id))).size;

export { waitingTitles };

/** "1 episode", "2 episodes". */
const plural = (n: number, one: string) => `${n} ${one}${n === 1 ? "" : "s"}`;

export { plural };

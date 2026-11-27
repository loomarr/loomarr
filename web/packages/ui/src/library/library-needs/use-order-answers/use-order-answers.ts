import { useState } from "react";

import type { LibraryNeedsAnswers, LibraryOrderNeed } from "../library-needs.type";
import { FILES, type OrderOption } from "../order-options";
import { useAnswers } from "../use-answers";

// The answers given to order needs: the shared lock-in, and what each said for its done line.
const useOrderAnswers = (
  orders: readonly LibraryOrderNeed[],
  onChooseOrder: LibraryNeedsAnswers["onChooseOrder"],
) => {
  const answers = useAnswers(orders);
  const [said, setSaid] = useState<Record<string, OrderOption>>({});
  const choose = (need: LibraryOrderNeed, option: OrderOption) => {
    setSaid((all) => ({ ...all, [need.id]: option }));
    answers.answer(need.id, "added", onChooseOrder(need.id, option.answer));
  };
  const doneOf = (id: string) => {
    const option = said[id];
    return answers.phases[id] && option ? { line: option.done, fromTmdb: option.key !== FILES } : undefined;
  };
  return { ...answers, choose, doneOf };
};

export { useOrderAnswers };

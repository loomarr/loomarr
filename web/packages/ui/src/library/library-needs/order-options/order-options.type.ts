import type { LibraryOrderAnswer } from "../library-needs.type";

interface OrderOption {
  key: string;
  answer: LibraryOrderAnswer;
  name: string;
  sub: string;
  fits: boolean;
  /** How the done line says it: "Harbor Lights {done}." */
  done: string;
}

export type { OrderOption };

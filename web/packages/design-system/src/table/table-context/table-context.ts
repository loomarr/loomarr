import { createContext, useContext } from "react";

import type { Density } from "../../tokens";

// What a table tells its cells: the density to set text at, and whether it is the dense one.
type TableContextValue = { dense: boolean; density: Density };

const TableContext = createContext<TableContextValue>({ dense: false, density: "pointer" });

const useTableContext = () => useContext(TableContext);

export { TableContext, useTableContext };

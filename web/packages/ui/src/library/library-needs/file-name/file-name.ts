/** The last part of a path: the name a file goes by. */
const fileName = (path: string) => path.split("/").filter(Boolean).pop() ?? path;

export { fileName };

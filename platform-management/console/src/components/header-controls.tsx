import { createContext, useContext, type ReactNode } from "react";

// Layout publishes the account and preference controls through this context so
// they render on the same row as each page's title, at the right edge.
const HeaderControlsContext = createContext<ReactNode>(null);

export const HeaderControlsProvider = HeaderControlsContext.Provider;

export function HeaderControls() {
  const controls = useContext(HeaderControlsContext);
  if (!controls) return null;
  return <div className="ml-auto flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2">{controls}</div>;
}

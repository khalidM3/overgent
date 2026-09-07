import { useCallback, useEffect, useState } from "react";
import { PanelLeftClose, PanelLeftOpen } from "lucide-react";

/** One shortcut contract shared by the live workroom and desktop entry shell. */
export function useSidebarCollapsed() {
  const [collapsed, setCollapsed] = useState(false);
  const toggle = useCallback(() => setCollapsed((current) => !current), []);

  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== "b" || event.repeat) return;
      event.preventDefault();
      toggle();
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [toggle]);

  return [collapsed, toggle] as const;
}

export function SidebarToggle({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  return <button
    className="icon-button sidebar-toggle"
    onClick={onToggle}
    aria-label={collapsed ? "Expand Projects sidebar" : "Collapse Projects sidebar"}
    aria-keyshortcuts="Meta+B Control+B"
    title={`${collapsed ? "Expand" : "Collapse"} Projects sidebar (⌘B)`}
  >{collapsed ? <PanelLeftOpen size={17} /> : <PanelLeftClose size={17} />}</button>;
}

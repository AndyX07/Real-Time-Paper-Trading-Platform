import { useEffect, useRef, useState } from "react";

interface SymbolPickerProps {
  symbol: string;
  symbols: string[];
  onSelect: (symbol: string) => void;
}

const MAX_RESULTS = 30;

export function SymbolPicker({ symbol, symbols, onSelect }: SymbolPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  const filtered = (
    query ? symbols.filter((s) => s.toLowerCase().includes(query.toLowerCase())) : symbols
  ).slice(0, MAX_RESULTS);

  function select(s: string) {
    onSelect(s);
    setOpen(false);
    setQuery("");
  }

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1.5 rounded-md bg-buy px-3 py-1.5 text-sm font-medium text-white"
      >
        {symbol}
        <span className="text-xs text-white/70">▾</span>
      </button>

      {open && (
        <div className="absolute left-0 top-full z-20 mt-1 w-64 rounded-md border border-border bg-panel shadow-lg">
          <input
            autoFocus
            type="text"
            placeholder="Search symbol..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-full border-b border-border bg-panel-alt px-3 py-2 text-sm text-text-primary outline-none"
          />
          <div className="max-h-72 overflow-y-auto">
            {symbols.length === 0 ? (
              <div className="px-3 py-2 text-sm text-text-muted">Loading symbols...</div>
            ) : filtered.length === 0 ? (
              <div className="px-3 py-2 text-sm text-text-muted">No matches.</div>
            ) : (
              filtered.map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => select(s)}
                  className={`block w-full px-3 py-1.5 text-left text-sm hover:bg-panel-alt ${
                    s === symbol ? "text-buy" : "text-text-primary"
                  }`}
                >
                  {s}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}

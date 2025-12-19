"use client";

import { useState, useEffect, useMemo, useRef } from "react";
import { Search } from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";
import { cn } from "@/lib/utils";
import useSWR from "swr";
import { useRouter } from "next/navigation";
import type { BackendSearchResponse } from "@/lib/api-types";

function useDebouncedValue<T>(value: T, delayMs: number) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(t);
  }, [value, delayMs]);
  return debounced;
}

async function fetchSearch(q: string): Promise<BackendSearchResponse> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(new Error("timeout")), 8_000);
  let res: Response;
  try {
    res = await fetch(`/api/search?q=${encodeURIComponent(q)}`, {
      method: "GET",
      headers: { accept: "application/json" },
      cache: "no-store",
      signal: controller.signal,
    });
  } catch (e) {
    if ((e as Error)?.name === "AbortError") throw new Error("timeout");
    throw e;
  } finally {
    clearTimeout(timeout);
  }
  const text = await res.text();
  if (!res.ok) throw new Error(text);
  return JSON.parse(text) as BackendSearchResponse;
}

export function SearchInput() {
  const [query, setQuery] = useState("");
  const [isFocused, setIsFocused] = useState(false);
  const router = useRouter();
  const containerRef = useRef<HTMLDivElement>(null);

  const debouncedQuery = useDebouncedValue(query.trim(), 250);
  const canSearch = debouncedQuery.length >= 2;

  const { data, isLoading } = useSWR(
    canSearch ? ["search", debouncedQuery] : null,
    ([, q]) => fetchSearch(q),
    {
      keepPreviousData: true,
      revalidateOnFocus: false,
      dedupingInterval: 750,
    },
  );

  const suggestions = useMemo(() => {
    return (data?.results || []).map((r) => {
      const label = r.global_name || r.nickname || r.username || r.user_id;
      return { id: r.user_id, label };
    });
  }, [data]);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsFocused(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const handleSelect = (id: string) => {
    router.push(`/${id}`);
    setIsFocused(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && query.trim()) {
      if (suggestions.length > 0) {
        handleSelect(suggestions[0].id);
        return;
      }
      // fallback: se o usuário colar um discord id direto
      if (/^\d{10,30}$/.test(query.trim())) {
        handleSelect(query.trim());
      }
    }
  };

  return (
    <div className="relative w-full max-w-md z-50" ref={containerRef}>
      <div
        className={cn(
          "relative flex items-center w-full h-12 rounded-full border transition-all duration-300",
          "glass bg-neutral-900/50",
          isFocused ? "border-neutral-700 shadow-[0_0_20px_rgba(0,0,0,0.3)] ring-1 ring-neutral-800" : "border-neutral-800"
        )}
      >
        <Search className="absolute left-4 w-4 h-4 text-neutral-400" suppressHydrationWarning />
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onFocus={() => setIsFocused(true)}
          onKeyDown={handleKeyDown}
          placeholder="buscar usuario..."
          className="w-full h-full bg-transparent pl-12 pr-4 text-sm text-neutral-200 placeholder-neutral-500 focus:outline-none rounded-full"
        />
      </div>

      <AnimatePresence>
        {isFocused && query.trim().length > 0 && (
          <motion.div
            initial={{ opacity: 0, y: 10, scale: 0.95 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 10, scale: 0.95 }}
            transition={{ duration: 0.2 }}
            className="absolute top-14 left-0 w-full rounded-2xl glass bg-neutral-900/80 border border-neutral-800 overflow-hidden shadow-2xl backdrop-blur-xl"
          >
            {isLoading ? (
              <div className="p-4 text-center text-xs text-neutral-500">buscando...</div>
            ) : canSearch && suggestions.length > 0 ? (
              <ul className="py-2">
                {suggestions.map((user) => (
                  <li key={user.id}>
                    <button
                      onClick={() => handleSelect(user.id)}
                      className="w-full px-4 py-3 flex items-center gap-3 hover:bg-white/5 transition-colors text-left"
                    >
                      <div className="w-8 h-8 rounded-full bg-neutral-800 flex items-center justify-center text-[10px] text-neutral-400 font-mono">
                        {user.label.slice(0, 2).toUpperCase()}
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-medium text-neutral-200">{user.label}</span>
                        <span className="text-xs text-neutral-500">id: {user.id}</span>
                      </div>
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <div className="p-4 text-center text-xs text-neutral-500">
                {query.trim().length < 2 ? "digite pelo menos 2 caracteres" : "nenhum resultado"}
              </div>
            )}
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}


"use client";

import { motion } from "framer-motion";
import { AlertTriangle, ExternalLink } from "lucide-react";
import { cn } from "@/lib/utils";

type Connection = {
  id: string;
  platform: string;
  label: string;
  url: string | null;
};

interface ConnectionsProps {
  connections: Connection[];
  possibleAltIds?: string[];
}

const platformIcons: Record<string, string> = {
    discord: "https://cdn.simpleicons.org/discord/white",
    spotify: "https://cdn.simpleicons.org/spotify/1DB954",
    steam: "https://cdn.simpleicons.org/steam/white",
    github: "https://cdn.simpleicons.org/github/white",
    twitter: "https://cdn.simpleicons.org/x/white",
};

export function Connections({ connections, possibleAltIds }: ConnectionsProps) {
  return (
    <div className="space-y-4">
      <h3 className="text-sm font-medium text-neutral-400">conexoes</h3>

      {possibleAltIds && possibleAltIds.length > 0 && (
        <div className="p-4 rounded-2xl glass border border-amber-900/30 bg-amber-900/5">
          <div className="flex items-center gap-2 text-amber-400/90">
            <AlertTriangle className="w-4 h-4" />
            <span className="text-sm font-medium">possible alt</span>
          </div>
          <div className="mt-2 text-xs text-neutral-300 font-mono break-all">
            {possibleAltIds.slice(0, 8).map((id, idx) => (
              <span key={id}>
                <a className="hover:underline" href={`/${id}`}>{id}</a>
                {idx < Math.min(possibleAltIds.length, 8) - 1 ? " · " : ""}
              </span>
            ))}
            {possibleAltIds.length > 8 && (
              <span className="text-neutral-500"> · +{possibleAltIds.length - 8}</span>
            )}
          </div>
        </div>
      )}

      <div className="flex flex-col gap-3">
        {connections.map((conn, index) => (
          <motion.a
            key={conn.id}
            href={conn.url || undefined}
            target={conn.url ? "_blank" : undefined}
            rel={conn.url ? "noopener noreferrer" : undefined}
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: index * 0.1 }}
            className={cn(
              "flex items-center justify-between p-3 rounded-xl glass border border-neutral-800/50 hover:bg-white/5 transition-all group",
              !conn.url && "opacity-90 cursor-default"
            )}
          >
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg bg-neutral-800 flex items-center justify-center p-1.5">
                 {/* Simple Icons CDN for brand icons */}
                 <img src={platformIcons[conn.platform] || platformIcons.discord} alt={conn.platform} className="w-full h-full object-contain opacity-70 group-hover:opacity-100 transition-opacity" />
              </div>
              <div className="flex flex-col">
                <span className="text-sm font-medium text-neutral-200 group-hover:text-white transition-colors">
                    {conn.label}
                </span>
                <span className="text-xs text-neutral-500 capitalize">{conn.platform}</span>
              </div>
            </div>

            {conn.url && (
                <ExternalLink className="w-4 h-4 text-neutral-600 group-hover:text-neutral-400 transition-colors" />
            )}
          </motion.a>
        ))}
      </div>
    </div>
  );
}


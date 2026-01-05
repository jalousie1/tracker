"use client";

import { motion } from "framer-motion";
import { AlertTriangle, ExternalLink, Calendar, Clock } from "lucide-react";
import { cn } from "@/lib/utils";
import { FormattedDate } from "./FormattedDate";

type Connection = {
  id: string;
  type: string;
  external_id: string;
  name: string;
  first_seen: string;
  last_seen: string;
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
  twitch: "https://cdn.simpleicons.org/twitch/9146FF",
  youtube: "https://cdn.simpleicons.org/youtube/FF0000",
  reddit: "https://cdn.simpleicons.org/reddit/FF4500",
  xbox: "https://cdn.simpleicons.org/xbox/107C10",
  playstation: "https://cdn.simpleicons.org/playstation/003791",
  battle_net: "https://cdn.simpleicons.org/battlenet/00AEFF",
  leagueoflegends: "https://cdn.simpleicons.org/leagueoflegends/white",
};

export function Connections({ connections, possibleAltIds }: ConnectionsProps) {
  if (connections.length === 0 && (!possibleAltIds || possibleAltIds.length === 0)) {
    return (
      <div className="py-10 text-center text-neutral-500 bg-neutral-900/10 rounded-2xl border border-dashed border-neutral-800">
        Nenhuma conexão detectada
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {possibleAltIds && possibleAltIds.length > 0 && (
        <div className="p-4 rounded-xl border border-amber-900/30 bg-amber-900/5 backdrop-blur-sm">
          <div className="flex items-center gap-2 text-amber-500 font-bold text-xs uppercase tracking-wider">
            <AlertTriangle className="w-4 h-4" />
            Possible Alt Accounts
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {possibleAltIds.slice(0, 10).map((id) => (
              <a
                key={id}
                href={`/${id}`}
                className="px-2 py-1 rounded bg-amber-500/10 text-amber-500 hover:bg-amber-500/20 transition-all font-mono text-[11px] border border-amber-500/20"
              >
                {id}
              </a>
            ))}
            {possibleAltIds.length > 10 && (
              <span className="text-[10px] text-neutral-500 self-center">+{possibleAltIds.length - 10} mais</span>
            )}
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 gap-3">
        {connections.map((conn, index) => {
          const platform = platformIcons[conn.type] ? conn.type : "discord";

          return (
            <motion.div
              key={`${conn.type}-${conn.external_id}`}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: index * 0.05 }}
              className="group relative overflow-hidden rounded-2xl border border-neutral-800/50 hover:border-neutral-700/50 transition-all bg-neutral-900/20 hover:bg-neutral-800/30 p-4"
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-4">
                  <div className="relative">
                    <div className="w-12 h-12 rounded-xl bg-neutral-900 border border-neutral-800 flex items-center justify-center p-2.5 group-hover:scale-105 transition-transform">
                      <img
                        src={platformIcons[conn.type] || platformIcons.discord}
                        alt={conn.type}
                        className="w-full h-full object-contain opacity-80 group-hover:opacity-100 transition-opacity"
                      />
                    </div>
                    <div className="absolute -bottom-1 -right-1 w-4 h-4 rounded-full bg-green-500 border-2 border-neutral-900" title="Recentemente vista" />
                  </div>

                  <div className="flex flex-col">
                    <span className="text-sm font-bold text-neutral-100 group-hover:text-white transition-colors">
                      {conn.name || 'Conexão Sem Nome'}
                    </span>
                    <span className="text-[10px] font-bold uppercase tracking-widest text-neutral-500">
                      {(conn.type || 'unknown').replace('_', ' ')}
                    </span>
                    <div className="mt-2 flex items-center gap-3">
                      <div className="flex items-center gap-1 text-[10px] text-neutral-500">
                        <Calendar className="w-3 h-3" />
                        <FormattedDate date={conn.first_seen} />
                      </div>
                      <div className="flex items-center gap-1 text-[10px] text-neutral-400">
                        <Clock className="w-3 h-3" />
                        <FormattedDate date={conn.last_seen} />
                      </div>
                    </div>
                  </div>
                </div>

                <div className="p-2 rounded-lg hover:bg-white/5 text-neutral-600 transition-colors">
                  <ExternalLink className="w-4 h-4" />
                </div>
              </div>

              {/* ID Badge */}
              <div className="absolute top-4 right-14 opacity-0 group-hover:opacity-100 transition-opacity">
                <span className="px-2 py-0.5 rounded bg-neutral-800 text-neutral-500 font-mono text-[9px]">
                  {conn.external_id}
                </span>
              </div>
            </motion.div>
          );
        })}
      </div>
    </div>
  );
}

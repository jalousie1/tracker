"use client";

import { motion } from "framer-motion";
import { Clock, Image as ImageIcon } from "lucide-react";

type TimelineEvent = {
  id: string;
  kind: "username" | "global_name" | "nickname" | "bio";
  at: string;
  from: string | null;
  to: string | null;
};

type Avatar = {
  id: string;
  url: string;
  at: string;
}

interface TimelineProps {
  events: TimelineEvent[];
  avatars: Avatar[];
}

function kindLabel(k: TimelineEvent["kind"]) {
  switch (k) {
    case "username":
      return "username";
    case "global_name":
      return "global name";
    case "nickname":
      return "nickname";
    case "bio":
      return "bio";
  }
}

export function Timeline({ events, avatars }: TimelineProps) {
  // Filtrar eventos redundantes ou onde values são idênticos
  const filteredEvents = events.filter(e => {
    // Se 'to' e 'from' são iguais, ignorar
    if (e.to === e.from) return false;

    // Se 'to' é vazio e 'from' não é, mas temos um evento anterior (mais recente no array DESC)
    // que já foi preenchido, isso pode ser um 'flicker' do gateway.
    // Mas o backend agora já filtra isso, então mantemos o filtro simples aqui.
    return true;
  });

  return (
    <div className="w-full space-y-8">
      {/* Avatar History Section */}
      {avatars.length > 0 && (
        <div className="space-y-3">
          <h3 className="text-sm font-medium text-neutral-400 flex items-center gap-2">
            <ImageIcon className="w-4 h-4" /> avatars anteriores
          </h3>
          <div className="flex gap-4 overflow-x-auto pb-4 snap-x">
            {avatars.map((avatar, index) => (
              <motion.div
                key={avatar.id}
                initial={{ opacity: 0, scale: 0.9 }}
                animate={{ opacity: 1, scale: 1 }}
                transition={{ delay: index * 0.1 }}
                className="flex-shrink-0 w-24 h-24 rounded-full overflow-hidden border border-neutral-800 bg-neutral-900 snap-center relative group"
              >
                <img src={avatar.url} alt="Avatar history" className="w-full h-full object-cover transition-transform duration-500 group-hover:scale-110" />
                <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                  <span className="text-[10px] text-white font-mono" suppressHydrationWarning>{new Date(avatar.at).toLocaleDateString()}</span>
                </div>
              </motion.div>
            ))}
          </div>
        </div>
      )}

      {/* Vertical Timeline */}
      <div className="relative pl-4 space-y-6">
        {/* Vertical Line */}
        <div className="absolute left-0 top-2 bottom-2 w-[1px] bg-neutral-800" />

        {filteredEvents.length > 0 ? filteredEvents.map((event, index) => (
          <motion.div
            key={event.id}
            initial={{ opacity: 0, x: -20 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ delay: index * 0.05 }}
            className="relative pl-6"
          >
            {/* Dot on line */}
            <div className="absolute left-[-4px] top-1.5 w-2 h-2 rounded-full bg-neutral-700 border border-neutral-900" />

            <div className="flex flex-col gap-1">
              <div className="flex items-center gap-2 text-xs text-neutral-500 font-mono mb-1">
                <Clock className="w-3 h-3" />
                <span suppressHydrationWarning>{new Date(event.at).toLocaleString()}</span>
              </div>
              <div className="glass p-4 rounded-xl border border-neutral-800/50 hover:border-neutral-700 transition-colors bg-neutral-900/30">
                <div className="text-[10px] text-neutral-500 font-bold uppercase tracking-wider mb-2">{kindLabel(event.kind)}</div>
                <div className="text-sm text-neutral-200 flex items-center gap-3">
                  <div className="min-w-0 flex-1">
                    {event.from ? (
                      <span className="text-neutral-500 line-through block truncate">{event.from}</span>
                    ) : (
                      <span className="text-neutral-500 block">vazio</span>
                    )}
                    <div className="flex items-center gap-2 mt-1">
                      <span className="text-neutral-600">→</span>
                      {event.to ? (
                        <span className="text-white font-bold truncate">{event.to}</span>
                      ) : (
                        <span className="text-neutral-500">vazio</span>
                      )}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </motion.div>
        )) : (
          <div className="text-sm text-neutral-500 pl-2">sem alterações relevantes no histórico</div>
        )}
      </div>
    </div>
  );
}

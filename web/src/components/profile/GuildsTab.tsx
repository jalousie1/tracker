"use client";

import { Users } from "lucide-react";
import { FormattedDate } from "@/components/FormattedDate";

interface GuildsTabProps {
    guilds: any[];
}

export function GuildsTab({ guilds }: GuildsTabProps) {
    return (
        <div className="space-y-4 animate-in fade-in slide-in-from-bottom-4 duration-500">
            <h3 className="text-sm font-medium text-neutral-400">guilds observed ({guilds.length})</h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {guilds.map((guild) => (
                    <div key={guild.id} className="glass p-4 rounded-xl flex items-center gap-4 hover:bg-white/5 transition-colors">
                        {guild.iconUrl ? (
                            <img src={guild.iconUrl} alt={guild.name} className="w-12 h-12 rounded-xl" />
                        ) : (
                            <div className="w-12 h-12 rounded-xl bg-neutral-800 flex items-center justify-center text-lg font-bold">
                                {guild.name.slice(0, 1)}
                            </div>
                        )}
                        <div className="flex-1 min-w-0">
                            <div className="font-medium text-white truncate">{guild.name}</div>
                            <div className="text-xs text-neutral-500 font-mono">{guild.id}</div>
                            <div className="text-[10px] text-neutral-400 mt-1">
                                Joined: <FormattedDate date={guild.joinedAt} includeTime={false} />
                                {guild.leftAt && (
                                    <> • Left: <FormattedDate date={guild.leftAt} includeTime={false} /></>
                                )}
                            </div>
                        </div>
                    </div>
                ))}
                {guilds.length === 0 && (
                    <div className="col-span-full py-20 text-center text-neutral-500 bg-neutral-900/20 rounded-2xl border border-dashed border-neutral-800">
                        No guilds observed yet
                    </div>
                )}
            </div>
        </div>
    );
}

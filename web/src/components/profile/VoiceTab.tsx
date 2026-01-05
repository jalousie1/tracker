"use client";

import { Mic, Phone, Users, Copy, Check, ChevronDown, User } from "lucide-react";
import { FormattedDate } from "@/components/FormattedDate";
import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { cn } from "@/lib/utils";

interface VoiceParticipant {
    user_id: string;
    username: string;
    avatar_hash?: string;
}

interface VoiceSession {
    id: number;
    channelName: string;
    guildName: string;
    joinedAt: string;
    leftAt?: string;
    duration?: string;
    participants?: VoiceParticipant[];
}

interface VoiceTabProps {
    voiceHistory: VoiceSession[];
}

export function VoiceTab({ voiceHistory }: VoiceTabProps) {
    const [copiedId, setCopiedId] = useState<string | null>(null);
    const [expandedSessions, setExpandedSessions] = useState<Record<number, boolean>>({});

    const handleCopyId = (e: React.MouseEvent, id: string) => {
        e.stopPropagation();
        navigator.clipboard.writeText(id);
        setCopiedId(id);
        setTimeout(() => setCopiedId(null), 2000);
    };

    const toggleExpand = (sessionId: number) => {
        setExpandedSessions(prev => ({
            ...prev,
            [sessionId]: !prev[sessionId]
        }));
    };

    const getAvatarUrl = (userId: string, hash?: string) => {
        if (!hash) return null;
        return `https://cdn.discordapp.com/avatars/${userId}/${hash}.png?size=64`;
    };

    return (
        <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
            <h3 className="text-sm font-medium text-neutral-400">recent voice activity</h3>
            <div className="space-y-4">
                {voiceHistory.map((v) => {
                    const isExpanded = !!expandedSessions[v.id];
                    const hasParticipants = v.participants && v.participants.length > 0;

                    return (
                        <div key={v.id} className="glass overflow-hidden rounded-2xl border border-neutral-800/50 hover:border-neutral-700/50 transition-all group bg-neutral-900/20">
                            <div className="p-5 flex flex-col md:flex-row md:items-center gap-4">
                                <div className={cn(
                                    "p-3 rounded-xl flex-shrink-0 transition-colors",
                                    v.leftAt ? "bg-neutral-800 text-neutral-500" : "bg-green-500/10 text-green-500"
                                )}>
                                    <Mic className="w-6 h-6" />
                                </div>

                                <div className="flex-1 min-w-0">
                                    <div className="flex items-center gap-2 flex-wrap">
                                        <h4 className="text-lg font-bold text-white truncate max-w-[200px] md:max-w-md">{v.channelName}</h4>
                                        {!v.leftAt && (
                                            <span className="px-2 py-0.5 rounded-full bg-green-500/20 text-green-500 text-[10px] uppercase font-bold tracking-wider animate-pulse border border-green-500/30">
                                                Running
                                            </span>
                                        )}
                                    </div>
                                    <div className="flex items-center gap-2 text-sm text-neutral-400 mt-0.5">
                                        <span className="flex items-center gap-1 font-medium">
                                            <Phone className="w-3 h-3 text-neutral-500" />
                                            {v.guildName}
                                        </span>
                                        <span className="text-neutral-700 font-mono">•</span>
                                        <div className="text-[11px] uppercase font-mono tracking-tight text-neutral-500">
                                            <FormattedDate date={v.joinedAt} />
                                            {v.leftAt ? (
                                                <> - <FormattedDate date={v.leftAt} /> • {v.duration}</>
                                            ) : (
                                                <> - Currently active</>
                                            )}
                                        </div>
                                    </div>
                                </div>

                                <div className="flex items-center gap-3">
                                    {hasParticipants && (
                                        <button
                                            onClick={() => toggleExpand(v.id)}
                                            className={cn(
                                                "flex items-center gap-2 px-3 py-2 rounded-xl border transition-all text-xs font-bold uppercase tracking-wider",
                                                isExpanded
                                                    ? "bg-white/10 border-neutral-600 text-white"
                                                    : "bg-neutral-800/50 border-neutral-800 text-neutral-400 hover:text-neutral-200 hover:border-neutral-700"
                                            )}
                                        >
                                            <Users className="w-4 h-4" />
                                            <span>{v.participants?.length} <span className="hidden sm:inline">participantes</span></span>
                                            <ChevronDown className={cn("w-4 h-4 transition-transform duration-300", isExpanded && "rotate-180")} />
                                        </button>
                                    )}
                                </div>
                            </div>

                            <AnimatePresence>
                                {isExpanded && hasParticipants && (
                                    <motion.div
                                        initial={{ height: 0, opacity: 0 }}
                                        animate={{ height: "auto", opacity: 1 }}
                                        exit={{ height: 0, opacity: 0 }}
                                        transition={{ duration: 0.3, ease: "easeInOut" }}
                                        className="overflow-hidden"
                                    >
                                        <div className="border-t border-neutral-800/50 bg-black/40 p-4 space-y-3">
                                            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
                                                {v.participants?.map((p) => (
                                                    <motion.div
                                                        key={p.user_id}
                                                        initial={{ opacity: 0, y: 5 }}
                                                        animate={{ opacity: 1, y: 0 }}
                                                        className="flex items-center gap-3 p-2.5 rounded-xl border border-neutral-800/30 bg-neutral-900/40 hover:bg-neutral-800/50 hover:border-neutral-700/50 transition-all group/p cursor-pointer"
                                                        onClick={(e) => handleCopyId(e, p.user_id)}
                                                    >
                                                        <div className="relative flex-shrink-0">
                                                            <div className="w-10 h-10 rounded-full bg-neutral-800 overflow-hidden ring-2 ring-neutral-900 group-hover/p:ring-neutral-700 transition-all">
                                                                {p.avatar_hash ? (
                                                                    <img src={getAvatarUrl(p.user_id, p.avatar_hash)!} alt={p.username} className="w-full h-full object-cover" />
                                                                ) : (
                                                                    <div className="w-full h-full flex items-center justify-center text-xs font-bold text-neutral-500">
                                                                        {p.username.charAt(0).toUpperCase()}
                                                                    </div>
                                                                )}
                                                            </div>
                                                            <div className="absolute -bottom-1 -right-1 w-4 h-4 rounded-full bg-neutral-900 border border-neutral-800 flex items-center justify-center opacity-0 group-hover/p:opacity-100 transition-opacity">
                                                                {copiedId === p.user_id ? (
                                                                    <Check className="w-2.5 h-2.5 text-green-500" />
                                                                ) : (
                                                                    <Copy className="w-2.5 h-2.5 text-neutral-500" />
                                                                )}
                                                            </div>
                                                        </div>
                                                        <div className="flex flex-col min-w-0">
                                                            <span className="text-sm font-bold text-neutral-200 group-hover/p:text-white truncate transition-colors">
                                                                {p.username}
                                                            </span>
                                                            <span className="text-[10px] font-mono text-neutral-600 group-hover/p:text-neutral-400 truncate transition-colors">
                                                                {p.user_id}
                                                            </span>
                                                        </div>
                                                    </motion.div>
                                                ))}
                                            </div>
                                        </div>
                                    </motion.div>
                                )}
                            </AnimatePresence>
                        </div>
                    );
                })}

                {voiceHistory.length === 0 && (
                    <div className="py-20 text-center text-neutral-500 bg-neutral-900/20 rounded-2xl border border-dashed border-neutral-800 flex flex-col items-center gap-4">
                        <Mic className="w-10 h-10 text-neutral-700" />
                        <span className="font-medium">No voice history recorded</span>
                    </div>
                )}
            </div>
        </div>
    );
}

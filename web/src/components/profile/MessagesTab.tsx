"use client";

import { MessageSquare, Paperclip, ExternalLink } from "lucide-react";
import { FormattedDate } from "@/components/FormattedDate";

interface MessagesTabProps {
    messages: any[];
}

export function MessagesTab({ messages }: MessagesTabProps) {
    return (
        <div className="space-y-4 animate-in fade-in slide-in-from-bottom-4 duration-500">
            <h3 className="text-sm font-medium text-neutral-400">recent messages ({messages.length})</h3>
            <div className="space-y-3">
                {messages.map((m) => (
                    <div key={m.id} className="glass p-4 rounded-xl space-y-2 hover:bg-white/5 transition-colors">
                        <div className="flex items-center justify-between">
                            <div className="flex items-center gap-2 text-xs font-medium text-neutral-400">
                                <span className="text-white">{m.guildName}</span>
                                <span>/</span>
                                <span className="text-blue-400">#{m.channelName}</span>
                            </div>
                            <div className="text-[10px] text-neutral-500 font-mono">
                                <FormattedDate date={m.createdAt} />
                            </div>
                        </div>
                        <p className="text-sm text-neutral-200 leading-relaxed break-words">{m.content}</p>
                        {(m.hasAttachments || m.hasEmbeds) && (
                            <div className="flex items-center gap-4 mt-2">
                                {m.hasAttachments && (
                                    <div className="flex items-center gap-1 text-[10px] text-neutral-500 font-medium">
                                        <Paperclip className="w-3 h-3" /> ATTACHMENT
                                    </div>
                                )}
                                {m.hasEmbeds && (
                                    <div className="flex items-center gap-1 text-[10px] text-neutral-500 font-medium">
                                        <ExternalLink className="w-3 h-3" /> EMBED
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                ))}
                {messages.length === 0 && (
                    <div className="py-20 text-center text-neutral-500 bg-neutral-900/20 rounded-2xl border border-dashed border-neutral-800">
                        No messages captured
                    </div>
                )}
            </div>
        </div>
    );
}

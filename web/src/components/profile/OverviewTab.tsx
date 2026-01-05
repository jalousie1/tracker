"use client";

import { useMemo } from "react";
import DOMPurify from "dompurify";
import { User, ShieldCheck } from "lucide-react";
import { Timeline } from "@/components/Timeline";
import { Connections } from "@/components/Connections";

interface OverviewTabProps {
    user: any;
    sanitizedBio: string;
    connections: any[];
    possibleAltIds?: string[];
    timeline: any;
}

export function OverviewTab({ user, sanitizedBio, connections, possibleAltIds, timeline }: OverviewTabProps) {
    return (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8 animate-in fade-in slide-in-from-bottom-4 duration-500">
            {/* Left Column: Bio & Connections */}
            <div className="space-y-8">
                {/* Bio Section */}
                {sanitizedBio && (
                    <div className="space-y-2">
                        <h3 className="text-sm font-medium text-neutral-400">bio</h3>
                        <div
                            className="glass p-4 rounded-xl text-neutral-300 text-sm leading-relaxed"
                            dangerouslySetInnerHTML={{ __html: sanitizedBio }}
                        />
                    </div>
                )}

                {/* Connections Section */}
                <div className="space-y-2">
                    <h3 className="text-sm font-medium text-neutral-400">connections</h3>
                    <Connections connections={connections} />
                </div>

                {/* Possible Alts Section */}
                {possibleAltIds && possibleAltIds.length > 0 && (
                    <div className="p-4 rounded-xl border border-yellow-500/20 bg-yellow-500/5">
                        <div className="flex items-center gap-2 text-yellow-500 mb-2">
                            <ShieldCheck className="w-4 h-4" />
                            <span className="text-sm font-medium">Alt Accounts Detected</span>
                        </div>
                        <div className="flex flex-wrap gap-2">
                            {possibleAltIds.map(id => (
                                <a
                                    key={id}
                                    href={`/${id}`}
                                    className="text-xs font-mono px-2 py-1 rounded bg-yellow-500/10 text-yellow-500 hover:bg-yellow-500/20 transition-colors"
                                >
                                    {id}
                                </a>
                            ))}
                        </div>
                    </div>
                )}
            </div>

            {/* Middle & Right Column: Timeline */}
            <div className="lg:col-span-2 space-y-4">
                <h3 className="text-sm font-medium text-neutral-400">history timeline</h3>
                <div className="glass p-6 rounded-2xl">
                    <Timeline events={timeline.events} avatars={timeline.avatars} />
                </div>
            </div>
        </div>
    );
}

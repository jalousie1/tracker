"use client";

import { useMemo, useState } from "react";
import DOMPurify from "dompurify";
import { Timeline } from "@/components/Timeline";
import { Connections } from "@/components/Connections";
import { BadgeCheck, LayoutGrid, Mic, Users, User, History, MessageSquare, Phone, Gamepad2, Palette } from "lucide-react";

interface ProfileCardProps {
  user: {
    id: string;
    status: string;
    createdAt: string;
    displayName: string;
    username: string | null;
    globalName: string | null;
    avatarUrl: string | null;
    bioText: string | null;
  };
  currentVoice: {
    channelName: string;
    guildName: string;
    joinedAt: string;
  } | null;
  lastPresence: {
    status: string;
    changedAt: string;
    guildId: string | null;
  } | null;
  lastActivity: {
    name: string;
    startedAt: string;
    endedAt: string | null;
    details: string | null;
    state: string | null;
    type: number;
  } | null;
  timeline: {
    events: Array<{
      id: string;
      kind: "username" | "global_name" | "nickname" | "bio";
      at: string;
      from: string | null;
      to: string | null;
    }>;
    avatars: Array<{ id: string; url: string; at: string }>;
  };
  connections: Array<{ id: string; platform: string; label: string; url: string | null }>;
  possibleAltIds?: string[];
  guilds: Array<{ id: string; name: string; joinedAt: string; leftAt: string | null; iconUrl: string | null }>;
  voiceHistory: Array<{ id: string; channelName: string; guildName: string; joinedAt: string; leftAt: string | null; duration: string; wasVideo: boolean; wasStreaming: boolean; wasMuted: boolean; wasDeafened: boolean }>;
  nicknames: Array<{ id: string; nickname: string; guildName: string; observedAt: string }>;
  messages: Array<{ id: string; content: string; guildName: string; channelName: string; createdAt: string; hasAttachments: boolean; hasEmbeds: boolean; replyToUserId: string | null }>;
  voicePartners: Array<{ id: string; partnerId: string; partnerName: string; partnerAvatarUrl: string | null; sessionCount: number; totalDuration: string; lastSessionAt: string }>;
  presenceHistory: Array<{ id: string; status: string; guildId: string | null; changedAt: string }>;
  activityHistory: Array<{ id: string; name: string; details: string | null; state: string | null; type: number; startedAt: string; endedAt: string | null; url: string | null; applicationId: string | null; spotifyTrackId: string | null; spotifyArtist: string | null; spotifyAlbum: string | null }>;
  bannerHistory: Array<{ id: string; bannerHash: string | null; bannerColor: string | null; urlCdn: string | null; changedAt: string }>;
  clanHistory: Array<{ id: string; clanTag: string | null; badge: string | null; changedAt: string }>;
  avatarDecorationHistory: Array<{ id: string; decorationAsset: string | null; decorationSkuId: string | null; changedAt: string }>;
}

function escapeHtml(s: string) {
  return s
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll("\"", "&quot;")
    .replaceAll("'", "&#039;");
}

export function ProfileCard({ user, currentVoice, lastPresence, lastActivity, timeline, connections, possibleAltIds, guilds, voiceHistory, nicknames, messages, voicePartners, activityHistory, bannerHistory, clanHistory, avatarDecorationHistory }: ProfileCardProps) {
  const [activeTab, setActiveTab] = useState<"overview" | "guilds" | "voice" | "nicknames" | "messages" | "partners" | "activity" | "cosmetics">("overview");

  const sanitizedBio = useMemo(() => {
    if (!user.bioText) return "";

    // sanitize html (xss) - tratando a bio como texto e convertendo \n -> <br/>
    const safeTextAsHtml = escapeHtml(user.bioText).replaceAll("\n", "<br/>");
    return DOMPurify.sanitize(safeTextAsHtml, {
      ALLOWED_TAGS: ["br", "b", "strong", "i", "em", "u", "p", "span", "code", "a"],
      ALLOWED_ATTR: ["href", "target", "rel"],
    });
  }, [user.bioText]);

  const tabs = [
    { id: "overview", label: "Overview", icon: LayoutGrid },
    { id: "guilds", label: "Guilds", icon: Users },
    { id: "voice", label: "Voice", icon: Mic },
    { id: "activity", label: "Activity", icon: Gamepad2 },
    { id: "nicknames", label: "Nicknames", icon: History },
    { id: "messages", label: "Messages", icon: MessageSquare },
    { id: "partners", label: "Partners", icon: Phone },
    { id: "cosmetics", label: "Cosmetics", icon: Palette },
  ] as const;

  return (
    <div className="w-full max-w-5xl mx-auto">
      {/* Glass Card Container */}
      <div className="glass rounded-3xl overflow-hidden shadow-2xl bg-neutral-950/40 min-h-[800px]">

        {/* Banner/Header Background */}
        <div className="h-32 bg-gradient-to-r from-neutral-900 via-neutral-800 to-neutral-900 opacity-50 relative">
          <div className="absolute inset-0 bg-[url('https://grainy-gradients.vercel.app/noise.svg')] opacity-20"></div>
        </div>

        <div className="px-8 pb-8">
          {/* Header Profile Info */}
          <div className="relative flex flex-col sm:flex-row items-end sm:items-end -mt-12 mb-8 gap-6">
            <div className="relative">
              <div className="w-24 h-24 rounded-full p-1 glass bg-neutral-900/50">
                {user.avatarUrl ? (
                  <img
                    src={user.avatarUrl}
                    alt={user.displayName}
                    className="w-full h-full rounded-full object-cover"
                  />
                ) : (
                  <div className="w-full h-full rounded-full bg-neutral-900 flex items-center justify-center text-neutral-400 font-mono text-xl">
                    {user.displayName.slice(0, 2).toUpperCase()}
                  </div>
                )}
              </div>
            </div>

            <div className="flex-1 mb-2">
              <div className="flex flex-col gap-1">
                <div className="flex items-center gap-2">
                  <h1 className="text-2xl font-bold text-white tracking-tight">
                    {user.globalName || user.username || user.displayName}
                  </h1>
                  {user.status === "active" && <BadgeCheck className="w-5 h-5 text-blue-400" />}
                </div>
                {user.globalName && user.username && (
                  <p className="text-neutral-400 font-medium">@{user.username}</p>
                )}
                <p className="text-sm text-neutral-500 font-mono">ID: {user.id}</p>

                {currentVoice && (
                  <div className="mt-2 inline-flex items-center gap-3 px-3 py-2 rounded-lg bg-green-500/10 border border-green-500/20 w-fit">
                    <div className="relative flex h-2 w-2">
                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75"></span>
                      <span className="relative inline-flex rounded-full h-2 w-2 bg-green-500"></span>
                    </div>
                    <div>
                      <div className="text-xs font-medium text-green-400">In Call</div>
                      <div className="text-[10px] text-green-500/80">
                        {currentVoice.channelName} • {currentVoice.guildName}
                      </div>
                    </div>
                  </div>
                )}

                {!currentVoice && lastPresence && (
                  <div className="mt-2 text-xs text-neutral-400" suppressHydrationWarning>
                    Presence: <span className="text-neutral-200">{lastPresence.status}</span> • {new Date(lastPresence.changedAt).toLocaleString("pt-BR")}
                  </div>
                )}
                {lastActivity && (
                  <div className="mt-1 text-xs text-neutral-400" suppressHydrationWarning>
                    Activity: <span className="text-neutral-200">{lastActivity.name}</span> • {new Date(lastActivity.startedAt).toLocaleString("pt-BR")}
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Tabs Navigation */}
          <div className="flex items-center gap-2 mb-8 border-b border-neutral-800 overflow-x-auto">
            {tabs.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.id;
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id)}
                  className={`
                      flex items-center gap-2 px-4 py-3 text-sm font-medium transition-colors relative
                      ${isActive ? "text-white" : "text-neutral-400 hover:text-neutral-200"}
                    `}
                >
                  <Icon className="w-4 h-4" />
                  {tab.label}
                  {isActive && (
                    <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-blue-500 shadow-[0_0_10px_rgba(59,130,246,0.5)]" />
                  )}
                </button>
              );
            })}
          </div>

          {/* Tab Content */}
          <div className="min-h-[400px]">
            {activeTab === "overview" && (
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-8 animate-in fade-in slide-in-from-bottom-4 duration-500">
                {/* Left Column: Bio & Connections */}
                <div className="space-y-8">
                  {/* Bio Section */}
                  {sanitizedBio && (
                    <div className="space-y-2">
                      <h3 className="text-sm font-medium text-neutral-400">bio</h3>
                      <div
                        className="text-sm text-neutral-300 leading-relaxed"
                        dangerouslySetInnerHTML={{ __html: sanitizedBio }}
                      />
                    </div>
                  )}

                  <Connections connections={connections} possibleAltIds={possibleAltIds} />
                </div>

                {/* Right Column: Timeline (Span 2) */}
                <div className="lg:col-span-2">
                  <Timeline events={timeline.events} avatars={timeline.avatars} />
                </div>
              </div>
            )}

            {activeTab === "guilds" && (
              <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                <h3 className="text-lg font-medium text-white mb-4">Guild History</h3>
                {guilds.length === 0 ? (
                  <p className="text-neutral-500">No guild history recorded.</p>
                ) : (
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {guilds.map((guild) => (
                      <div key={guild.id} className="p-4 rounded-xl bg-neutral-900/50 border border-neutral-800 flex items-center gap-4">
                        {guild.iconUrl ? (
                          <img src={guild.iconUrl} alt={guild.name} className="w-12 h-12 rounded-full" />
                        ) : (
                          <div className="w-12 h-12 rounded-full bg-neutral-800 flex items-center justify-center text-neutral-500">
                            <Users className="w-6 h-6" />
                          </div>
                        )}
                        <div>
                          <div className="font-medium text-white">{guild.name}</div>
                          <div className="text-xs text-neutral-500 font-mono">ID: {guild.id}</div>
                          <div className="text-xs text-neutral-400 mt-1">
                            <span suppressHydrationWarning>Joined: {new Date(guild.joinedAt).toLocaleDateString()}</span>
                            {guild.leftAt && <span suppressHydrationWarning> • Left: {new Date(guild.leftAt).toLocaleDateString()}</span>}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === "voice" && (
              <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                <h3 className="text-lg font-medium text-white mb-4">Voice Channel History</h3>
                {voiceHistory.length === 0 ? (
                  <p className="text-neutral-500">No voice history recorded.</p>
                ) : (
                  <div className="space-y-2">
                    {voiceHistory.map((voice) => (
                      <div key={voice.id} className="p-3 rounded-lg bg-neutral-900/30 border border-neutral-800/50 flex items-center justify-between hover:bg-neutral-900/50 transition-colors">
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-full bg-neutral-800 flex items-center justify-center text-neutral-500">
                            <Mic className="w-4 h-4" />
                          </div>
                          <div>
                            <div className="text-sm font-medium text-neutral-200">{voice.channelName}</div>
                            <div className="text-xs text-neutral-500">{voice.guildName}</div>
                            <div className="flex gap-2 mt-1">
                              {voice.wasVideo && <span className="text-[10px] bg-blue-500/20 text-blue-400 px-1.5 rounded">Video</span>}
                              {voice.wasStreaming && <span className="text-[10px] bg-purple-500/20 text-purple-400 px-1.5 rounded">Stream</span>}
                              {voice.wasMuted && <span className="text-[10px] bg-red-500/20 text-red-400 px-1.5 rounded">Muted</span>}
                              {voice.wasDeafened && <span className="text-[10px] bg-orange-500/20 text-orange-400 px-1.5 rounded">Deaf</span>}
                            </div>
                          </div>
                        </div>
                        <div className="text-right">
                          <div className="text-sm font-mono text-blue-400">{voice.duration}</div>
                          <div className="text-xs text-neutral-500" suppressHydrationWarning>
                            {new Date(voice.joinedAt).toLocaleString()}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === "nicknames" && (
              <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                <h3 className="text-lg font-medium text-white mb-4">Nickname History</h3>
                {nicknames.length === 0 ? (
                  <p className="text-neutral-500">No nickname history recorded.</p>
                ) : (
                  <div className="space-y-2">
                    {nicknames.map((nick) => (
                      <div key={nick.id} className="p-3 rounded-lg bg-neutral-900/30 border border-neutral-800/50 flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-full bg-neutral-800 flex items-center justify-center text-neutral-500">
                            <User className="w-4 h-4" />
                          </div>
                          <div>
                            <div className="text-sm font-medium text-neutral-200">{nick.nickname}</div>
                            <div className="text-xs text-neutral-500">{nick.guildName}</div>
                          </div>
                        </div>
                        <div className="text-xs text-neutral-500 font-mono" suppressHydrationWarning>
                          {new Date(nick.observedAt).toLocaleString()}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === "messages" && (
              <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                <h3 className="text-lg font-medium text-white mb-4">Message History</h3>
                {messages.length === 0 ? (
                  <p className="text-neutral-500">No message history recorded.</p>
                ) : (
                  <div className="space-y-3">
                    {messages.map((msg) => (
                      <div key={msg.id} className="p-4 rounded-lg bg-neutral-900/30 border border-neutral-800/50">
                        <div className="flex items-start justify-between gap-4">
                          <div className="flex-1 min-w-0">
                            {msg.content ? (
                              <p className="text-sm text-neutral-200 break-words whitespace-pre-wrap">
                                {msg.content}
                              </p>
                            ) : (
                              <p className="text-sm text-neutral-500 italic">
                                {msg.hasAttachments ? "[Attachment]" : msg.hasEmbeds ? "[Embed]" : "[No content]"}
                              </p>
                            )}
                            {msg.replyToUserId && (
                              <p className="text-xs text-neutral-500 mt-1">
                                ↩ Reply to: {msg.replyToUserId}
                              </p>
                            )}
                          </div>
                          <div className="flex items-center gap-2 flex-shrink-0">
                            {msg.hasAttachments && (
                              <span className="px-2 py-0.5 text-xs bg-blue-500/20 text-blue-400 rounded">📎</span>
                            )}
                            {msg.hasEmbeds && (
                              <span className="px-2 py-0.5 text-xs bg-purple-500/20 text-purple-400 rounded">📋</span>
                            )}
                          </div>
                        </div>
                        <div className="flex items-center justify-between mt-3 pt-3 border-t border-neutral-800/50">
                          <div className="flex items-center gap-2 text-xs text-neutral-500">
                            <MessageSquare className="w-3 h-3" />
                            <span className="text-neutral-400">#{msg.channelName}</span>
                            <span>•</span>
                            <span>{msg.guildName}</span>
                          </div>
                          <div className="text-xs text-neutral-500 font-mono" suppressHydrationWarning>
                            {new Date(msg.createdAt).toLocaleString("pt-BR", {
                              day: "2-digit",
                              month: "2-digit",
                              year: "numeric",
                              hour: "2-digit",
                              minute: "2-digit",
                            })}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === "partners" && (
              <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                <h3 className="text-lg font-medium text-white mb-4">Most Called Partners</h3>
                {voicePartners.length === 0 ? (
                  <p className="text-neutral-500">No call partner history recorded.</p>
                ) : (
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {voicePartners.map((partner, idx) => (
                      <div key={partner.id} className="p-4 rounded-xl bg-neutral-900/50 border border-neutral-800 flex items-center gap-4">
                        {partner.partnerAvatarUrl ? (
                          <img src={partner.partnerAvatarUrl} alt={partner.partnerName} className="w-10 h-10 rounded-full object-cover" />
                        ) : (
                          <div className="w-10 h-10 rounded-full bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center text-white font-bold text-sm">
                            #{idx + 1}
                          </div>
                        )}
                        <div className="flex-1 min-w-0">
                          <div className="font-medium text-white truncate">{partner.partnerName}</div>
                          <div className="text-xs text-neutral-500 font-mono">ID: {partner.partnerId}</div>
                          <div className="flex items-center gap-3 mt-1 text-xs text-neutral-400">
                            <span className="flex items-center gap-1">
                              <Phone className="w-3 h-3" />
                              {partner.sessionCount} calls
                            </span>
                            <span>•</span>
                            <span className="text-blue-400 font-mono">{partner.totalDuration}</span>
                          </div>
                        </div>
                        <div className="text-right text-xs text-neutral-500">
                          Last call:
                          <br />
                          <span className="font-mono" suppressHydrationWarning>
                            {new Date(partner.lastSessionAt).toLocaleDateString("pt-BR")}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === "activity" && (
              <div className="animate-in fade-in slide-in-from-bottom-4 duration-500">
                <h3 className="text-lg font-medium text-white mb-4">Activity History</h3>
                {activityHistory.length === 0 ? (
                  <p className="text-neutral-500">No activity history recorded.</p>
                ) : (
                  <div className="space-y-3">
                    {activityHistory.map((a) => (
                      <div key={a.id} className="p-4 rounded-lg bg-neutral-900/30 border border-neutral-800/50">
                        <div className="flex justify-between items-start mb-2">
                          <div className="font-medium text-white">{a.name}</div>
                          <div className="text-xs text-neutral-500 font-mono" suppressHydrationWarning>
                            {new Date(a.startedAt).toLocaleString()}
                          </div>
                        </div>
                        {a.details && <div className="text-sm text-neutral-300">{a.details}</div>}
                        {a.state && <div className="text-sm text-neutral-400">{a.state}</div>}
                        {(a.spotifyTrackId || a.spotifyArtist) && (
                          <div className="mt-2 pt-2 border-t border-neutral-800/50 text-xs text-green-400 flex items-center gap-2">
                            <span className="i-lucide-music w-3 h-3" />
                            {a.spotifyArtist} - {a.spotifyAlbum}
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {activeTab === "cosmetics" && (
              <div className="animate-in fade-in slide-in-from-bottom-4 duration-500 space-y-8">

                {/* Banners */}
                <div>
                  <h3 className="text-lg font-medium text-white mb-4">Banner History</h3>
                  {bannerHistory.length === 0 ? (
                    <p className="text-neutral-500 text-sm">No banner history.</p>
                  ) : (
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                      {bannerHistory.map((b) => (
                        <div key={b.id} className="rounded-xl overflow-hidden border border-neutral-800 bg-neutral-900/50">
                          {b.urlCdn ? (
                            <img src={b.urlCdn} alt="Banner" className="w-full h-32 object-cover" />
                          ) : (
                            <div className="w-full h-32" style={{ backgroundColor: b.bannerColor || '#000' }} />
                          )}
                          <div className="p-3 text-xs text-neutral-500 font-mono" suppressHydrationWarning>
                            Changed: {new Date(b.changedAt).toLocaleDateString()}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Avatar Decorations */}
                <div>
                  <h3 className="text-lg font-medium text-white mb-4">Avatar Decorations</h3>
                  {avatarDecorationHistory.length === 0 ? (
                    <p className="text-neutral-500 text-sm">No decoration history.</p>
                  ) : (
                    <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
                      {avatarDecorationHistory.map((ad) => (
                        <div key={ad.id} className="p-4 rounded-xl bg-neutral-900/50 border border-neutral-800 flex flex-col items-center gap-2">
                          <div className="text-xs text-neutral-400 font-mono truncate w-full text-center">
                            {ad.decorationSkuId || 'Unknown SKU'}
                          </div>
                          <div className="text-[10px] text-neutral-600" suppressHydrationWarning>
                            {new Date(ad.changedAt).toLocaleDateString()}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                {/* Clan Tags */}
                <div>
                  <h3 className="text-lg font-medium text-white mb-4">Clan History</h3>
                  {clanHistory.length === 0 ? (
                    <p className="text-neutral-500 text-sm">No clan history.</p>
                  ) : (
                    <div className="space-y-2">
                      {clanHistory.map((c) => (
                        <div key={c.id} className="p-3 rounded-lg bg-neutral-900/30 border border-neutral-800/50 flex items-center justify-between">
                          <div className="flex items-center gap-2">
                            {c.badge && <span className="text-xl">{c.badge}</span>}
                            <span className="font-medium text-neutral-200">{c.clanTag || 'No Tag'}</span>
                          </div>
                          <div className="text-xs text-neutral-500 font-mono" suppressHydrationWarning>
                            {new Date(c.changedAt).toLocaleDateString()}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}


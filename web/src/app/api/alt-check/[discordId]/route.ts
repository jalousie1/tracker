import { proxyGet } from "../../_proxy";

export const dynamic = "force-dynamic";

export async function GET(req: Request, ctx: { params: Promise<{ discordId: string }> }) {
  const { discordId } = await ctx.params;
  return proxyGet(`/alt-check/${encodeURIComponent(discordId)}`, req.signal);
}



package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Cores ANSI para terminal
const (
	Reset = "\033[0m"
	Bold  = "\033[1m"
	Dim   = "\033[2m"

	// Cores de texto
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	Gray    = "\033[90m"

	// Cores brilhantes
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"

	// Background
	BgRed    = "\033[41m"
	BgGreen  = "\033[42m"
	BgYellow = "\033[43m"
	BgBlue   = "\033[44m"
)

// Emojis para diferentes tipos de log
const (
	EmojiInfo    = "ℹ️ "
	EmojiSuccess = "✅"
	EmojiWarning = "⚠️ "
	EmojiError   = "❌"
	EmojiDebug   = "🔍"
	EmojiServer  = "🖥️ "
	EmojiGuild   = "🏰"
	EmojiUser    = "👤"
	EmojiToken   = "🔑"
	EmojiVoice   = "🎤"
	EmojiMessage = "💬"
	EmojiOnline  = "🟢"
	EmojiOffline = "🔴"
	EmojiScrape  = "🔄"
	EmojiDB      = "💾"
	EmojiAPI     = "🌐"
	EmojiGateway = "🔌"
)

// PrettyHandler é um handler customizado com cores e formatação bonita
type PrettyHandler struct {
	opts   slog.HandlerOptions
	mu     *sync.Mutex
	out    io.Writer
	attrs  []slog.Attr
	groups []string
}

func NewPrettyHandler(out io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &PrettyHandler{
		opts: *opts,
		mu:   &sync.Mutex{},
		out:  out,
	}
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Timestamp
	timeStr := r.Time.Format("15:04:05")

	// Level com cor e emoji
	levelStr, levelColor, emoji := h.getLevelInfo(r.Level)

	// Mensagem formatada
	msg := h.formatMessage(r.Message)

	// Linha principal
	line := fmt.Sprintf("%s%s%s %s%s%s %s%s%s %s",
		Gray, timeStr, Reset,
		levelColor, emoji, levelStr, Reset,
		Bold, msg, Reset,
	)

	// Atributos
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.formatAttr(a))
		return true
	})

	// Adicionar attrs do handler
	for _, a := range h.attrs {
		attrs = append(attrs, h.formatAttr(a))
	}

	if len(attrs) > 0 {
		line += " " + Gray + strings.Join(attrs, " ") + Reset
	}

	fmt.Fprintln(h.out, line)
	return nil
}

func (h *PrettyHandler) getLevelInfo(level slog.Level) (string, string, string) {
	switch {
	case level >= slog.LevelError:
		return "ERROR", BrightRed, EmojiError
	case level >= slog.LevelWarn:
		return "WARN ", BrightYellow, EmojiWarning
	case level >= slog.LevelInfo:
		return "INFO ", BrightCyan, EmojiInfo
	default:
		return "DEBUG", Gray, EmojiDebug
	}
}

func (h *PrettyHandler) formatMessage(msg string) string {
	// Adicionar emojis baseado no conteúdo da mensagem
	msgLower := strings.ToLower(msg)

	// Substituir underscores por espaços para melhor legibilidade
	msg = strings.ReplaceAll(msg, "_", " ")

	// Capitalizar primeira letra
	if len(msg) > 0 {
		msg = strings.ToUpper(msg[:1]) + msg[1:]
	}

	// Adicionar emoji contextual
	switch {
	case strings.Contains(msgLower, "guild") && strings.Contains(msgLower, "connect"):
		return EmojiGuild + " " + msg
	case strings.Contains(msgLower, "gateway"):
		return EmojiGateway + " " + msg
	case strings.Contains(msgLower, "token"):
		return EmojiToken + " " + msg
	case strings.Contains(msgLower, "scrape") || strings.Contains(msgLower, "scraping"):
		return EmojiScrape + " " + msg
	case strings.Contains(msgLower, "voice") || strings.Contains(msgLower, "call"):
		return EmojiVoice + " " + msg
	case strings.Contains(msgLower, "message"):
		return EmojiMessage + " " + msg
	case strings.Contains(msgLower, "user"):
		return EmojiUser + " " + msg
	case strings.Contains(msgLower, "server") || strings.Contains(msgLower, "started") || strings.Contains(msgLower, "listening"):
		return EmojiServer + " " + msg
	case strings.Contains(msgLower, "database") || strings.Contains(msgLower, "db") || strings.Contains(msgLower, "saved"):
		return EmojiDB + " " + msg
	case strings.Contains(msgLower, "api") || strings.Contains(msgLower, "http") || strings.Contains(msgLower, "request"):
		return EmojiAPI + " " + msg
	case strings.Contains(msgLower, "online"):
		return EmojiOnline + " " + msg
	case strings.Contains(msgLower, "offline") || strings.Contains(msgLower, "disconnect"):
		return EmojiOffline + " " + msg
	case strings.Contains(msgLower, "success") || strings.Contains(msgLower, "completed") || strings.Contains(msgLower, "connected"):
		return EmojiSuccess + " " + msg
	}

	return msg
}

func (h *PrettyHandler) formatAttr(a slog.Attr) string {
	key := a.Key
	val := a.Value.String()

	// Cores especiais para certas chaves
	keyColor := Cyan
	valColor := White

	switch key {
	case "error", "err":
		keyColor = Red
		valColor = BrightRed
	case "guild_id", "guild_name", "guilds_count":
		keyColor = Magenta
		valColor = BrightMagenta
	case "user_id", "user", "username":
		keyColor = Blue
		valColor = BrightBlue
	case "token_id", "token":
		keyColor = Yellow
		valColor = BrightYellow
	case "count", "total", "scraped", "saved":
		keyColor = Green
		valColor = BrightGreen
	case "duration", "time", "elapsed":
		keyColor = Gray
		valColor = White
	}

	return fmt.Sprintf("%s%s%s=%s%s%s", keyColor, key, Reset, valColor, val, Reset)
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newH := &PrettyHandler{
		opts:   h.opts,
		mu:     h.mu,
		out:    h.out,
		attrs:  append(h.attrs, attrs...),
		groups: h.groups,
	}
	return newH
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	newH := &PrettyHandler{
		opts:   h.opts,
		mu:     h.mu,
		out:    h.out,
		attrs:  h.attrs,
		groups: append(h.groups, name),
	}
	return newH
}

// New cria um novo logger com formatação bonita
func New(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	// Usar o handler bonito com cores
	h := NewPrettyHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})

	return slog.New(h)
}

// NewJSON cria um logger JSON para produção
func NewJSON(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})
	return slog.New(h)
}

func MaskToken(tok string) string {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return ""
	}
	if len(tok) <= 8 {
		return "***"
	}
	return tok[:3] + "***" + tok[len(tok)-3:]
}

// PrintBanner imprime um banner bonito ao iniciar o servidor
func PrintBanner() {
	banner := `
` + BrightCyan + `
  ╔═══════════════════════════════════════════════════════════════╗
  ║` + Reset + Bold + `    ` + BrightMagenta + `🔍 IDENTITY ARCHIVE` + Reset + BrightCyan + `                                   ║
  ║` + Reset + `    ` + Gray + `Discord User Tracking System` + Reset + BrightCyan + `                           ║
  ╠═══════════════════════════════════════════════════════════════╣
  ║` + Reset + `    ` + Green + EmojiServer + ` Server Starting...` + Reset + BrightCyan + `                              ║
  ╚═══════════════════════════════════════════════════════════════╝
` + Reset

	fmt.Println(banner)
}

// PrintStartupInfo imprime informações de inicialização
func PrintStartupInfo(port string, dbConnected bool, tokensCount int) {
	fmt.Println()
	fmt.Printf("  %s%s API Server%s\n", Bold, EmojiAPI, Reset)
	fmt.Printf("  %s├─%s Port: %s%s%s\n", Gray, Reset, BrightGreen, port, Reset)
	fmt.Printf("  %s├─%s Database: %s\n", Gray, Reset, statusString(dbConnected))
	fmt.Printf("  %s└─%s Tokens: %s%d%s active\n", Gray, Reset, BrightYellow, tokensCount, Reset)
	fmt.Println()

	if dbConnected && tokensCount > 0 {
		fmt.Printf("  %s%s Ready to track!%s\n", BrightGreen, EmojiSuccess, Reset)
	} else if !dbConnected {
		fmt.Printf("  %s%s Database not connected!%s\n", BrightRed, EmojiError, Reset)
	} else {
		fmt.Printf("  %s%s No tokens configured%s\n", BrightYellow, EmojiWarning, Reset)
	}
	fmt.Println()
}

func statusString(ok bool) string {
	if ok {
		return fmt.Sprintf("%s%s Connected%s", BrightGreen, EmojiOnline, Reset)
	}
	return fmt.Sprintf("%s%s Disconnected%s", BrightRed, EmojiOffline, Reset)
}

// PrintGatewayStatus imprime status das conexões gateway
func PrintGatewayStatus(connections int, guilds int) {
	fmt.Println()
	fmt.Printf("  %s%s Gateway Status%s\n", Bold, EmojiGateway, Reset)
	fmt.Printf("  %s├─%s Connections: %s%d%s\n", Gray, Reset, BrightCyan, connections, Reset)
	fmt.Printf("  %s└─%s Guilds: %s%d%s\n", Gray, Reset, BrightMagenta, guilds, Reset)
	fmt.Println()
}

// FormatDuration formata uma duração de forma legível
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// LogBox imprime uma caixa com mensagem
func LogBox(title string, lines []string) {
	maxLen := len(title)
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}

	border := strings.Repeat("─", maxLen+2)

	fmt.Printf("\n  %s╭%s╮%s\n", Cyan, border, Reset)
	fmt.Printf("  %s│%s %s%s%s%s │%s\n", Cyan, Reset, Bold, title, strings.Repeat(" ", maxLen-len(title)), Cyan, Reset)
	fmt.Printf("  %s├%s┤%s\n", Cyan, border, Reset)

	for _, line := range lines {
		fmt.Printf("  %s│%s %s%s %s│%s\n", Cyan, Reset, line, strings.Repeat(" ", maxLen-len(line)), Cyan, Reset)
	}

	fmt.Printf("  %s╰%s╯%s\n\n", Cyan, border, Reset)
}

// ProgressBar retorna uma barra de progresso como string
func ProgressBar(current, total int, width int) string {
	if total == 0 {
		return ""
	}

	percent := float64(current) / float64(total)
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("%s[%s%s%s]%s %.1f%%", Gray, BrightGreen, bar, Gray, Reset, percent*100)
}

// PrintScrapeProgress imprime progresso de scraping
func PrintScrapeProgress(guildName string, current, total int, membersScraped int) {
	bar := ProgressBar(current, total, 20)
	fmt.Printf("\r  %s%s%s %s %s%d%s members %s",
		EmojiScrape, BrightCyan, guildName, bar, BrightYellow, membersScraped, Reset, Reset)
}

// PrintGuildInfo imprime informações de um guild
func PrintGuildInfo(name string, memberCount int, channelCount int, roleCount int) {
	fmt.Printf("  %s%s %s%s%s\n", EmojiGuild, Bold, name, Reset, "")
	fmt.Printf("  %s├─%s Members: %s%d%s\n", Gray, Reset, BrightCyan, memberCount, Reset)
	fmt.Printf("  %s├─%s Channels: %s%d%s\n", Gray, Reset, BrightMagenta, channelCount, Reset)
	fmt.Printf("  %s└─%s Roles: %s%d%s\n", Gray, Reset, BrightYellow, roleCount, Reset)
}

// PrintTokenInfo imprime informações de um token
func PrintTokenInfo(tokenID int64, guildCount int, status string) {
	statusColor := BrightGreen
	statusEmoji := EmojiOnline
	if status != "online" {
		statusColor = BrightRed
		statusEmoji = EmojiOffline
	}

	fmt.Printf("  %s Token #%d\n", EmojiToken, tokenID)
	fmt.Printf("  %s├─%s Status: %s%s %s%s\n", Gray, Reset, statusColor, statusEmoji, status, Reset)
	fmt.Printf("  %s└─%s Guilds: %s%d%s\n", Gray, Reset, BrightMagenta, guildCount, Reset)
}

// PrintEventStats imprime estatísticas de eventos
func PrintEventStats(processed, queued, errors int) {
	fmt.Printf("\n  %s%s Event Stats%s\n", Bold, EmojiMessage, Reset)
	fmt.Printf("  %s├─%s Processed: %s%d%s\n", Gray, Reset, BrightGreen, processed, Reset)
	fmt.Printf("  %s├─%s Queued: %s%d%s\n", Gray, Reset, BrightYellow, queued, Reset)
	fmt.Printf("  %s└─%s Errors: %s%d%s\n", Gray, Reset, BrightRed, errors, Reset)
	fmt.Println()
}

// PrintSeparator imprime uma linha separadora
func PrintSeparator() {
	fmt.Printf("  %s%s%s\n", Gray, strings.Repeat("─", 50), Reset)
}

// PrintSection imprime um cabeçalho de seção
func PrintSection(title string) {
	fmt.Printf("\n  %s%s %s%s\n", Bold, "▸", title, Reset)
	PrintSeparator()
}

// FormatNumber formata um número grande de forma legível
func FormatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1000000)
}

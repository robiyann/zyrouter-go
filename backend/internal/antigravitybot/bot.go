// Package antigravitybot implements the standalone Telegram quota bot.
package antigravitybot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"zyrouter/backend/internal/antigravityquota"
)

const (
	telegramAPIBase = "https://api.telegram.org/bot"
	longPollTimeout = 25
	messageLimit    = 3900
)

type service interface {
	Snapshot(context.Context, bool) (*antigravityquota.Snapshot, error)
}

type Bot struct {
	token         string
	allowedUsers  map[int64]struct{}
	quota         service
	client        *http.Client
	minInterval   time.Duration
	lastRequestMu sync.Mutex
	lastRequest   map[int64]time.Time
}

type user struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type message struct {
	Chat chat   `json:"chat"`
	From *user  `json:"from"`
	Text string `json:"text"`
}

type update struct {
	UpdateID int64    `json:"update_id"`
	Message  *message `json:"message"`
}

type telegramResponse[T any] struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	Result      T      `json:"result"`
}

type sentMessage struct {
	MessageID int64 `json:"message_id"`
}

// ParseAllowedUserIDs parses a comma-separated Telegram user ID allowlist.
func ParseAllowedUserIDs(raw string) (map[int64]struct{}, error) {
	allowed := make(map[int64]struct{})
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		id, err := strconv.ParseInt(item, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid Telegram user ID %q", item)
		}
		allowed[id] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("Telegram user ID allowlist is empty")
	}
	return allowed, nil
}

type botInfo struct {
	Username string `json:"username"`
}

// New creates a quota bot. allowedUsers must be non-empty; exposing all
// provider accounts to arbitrary Telegram users would be unsafe.
func New(token string, allowedUsers map[int64]struct{}, quota service) *Bot {
	return &Bot{
		token:        strings.TrimSpace(token),
		allowedUsers: allowedUsers,
		quota:        quota,
		client:       &http.Client{Timeout: 35 * time.Second},
		minInterval:  10 * time.Second,
		lastRequest:  make(map[int64]time.Time),
	}
}

// Run starts Telegram long polling until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	if b == nil || b.token == "" {
		return fmt.Errorf("Telegram bot token is required")
	}
	if len(b.allowedUsers) == 0 {
		return fmt.Errorf("at least one allowed Telegram user ID is required")
	}
	if b.quota == nil {
		return fmt.Errorf("quota service is required")
	}
	if _, err := call[botInfo](b, ctx, "getMe", nil); err != nil {
		return fmt.Errorf("verify Telegram bot token: %w", err)
	}
	if _, err := call[json.RawMessage](b, ctx, "deleteWebhook", url.Values{"drop_pending_updates": {"false"}}); err != nil {
		return fmt.Errorf("delete Telegram webhook: %w", err)
	}

	var offset int64
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, item := range updates {
			if item.UpdateID >= offset {
				offset = item.UpdateID + 1
			}
			if item.Message != nil {
				b.handleMessage(ctx, item.Message)
			}
		}
	}
}

func (b *Bot) handleMessage(ctx context.Context, msg *message) {
	if msg.From == nil || msg.Chat.Type != "private" {
		return
	}
	if _, ok := b.allowedUsers[msg.From.ID]; !ok {
		return
	}

	command, argument := parseCommand(msg.Text)
	switch command {
	case "", "start", "help":
		b.send(ctx, msg.Chat.ID, "🚀 Antigravity Quota Bot\n\n/quota — tampilkan quota 5-hour, weekly, dan per-model\n/quota <email|id> — tampilkan satu akun\n/weekly — tampilkan quota weekly saja\n/refresh — paksa ambil data terbaru")
	case "quota":
		b.replyQuota(ctx, msg.Chat.ID, msg.From.ID, false, argument)
	case "weekly":
		b.replyQuota(ctx, msg.Chat.ID, msg.From.ID, true, argument)
	case "refresh":
		b.replyQuota(ctx, msg.Chat.ID, msg.From.ID, false, "refresh")
	default:
		b.send(ctx, msg.Chat.ID, "Perintah tidak dikenal. Gunakan /help.")
	}
}

func (b *Bot) replyQuota(ctx context.Context, chatID, userID int64, weeklyOnly bool, argument string) {
	if !b.allowRequest(userID) {
		b.send(ctx, chatID, "Tunggu beberapa detik sebelum meminta quota lagi.")
		return
	}
	loadingID := b.send(ctx, chatID, "⏳ Sedang mengambil quota Antigravity...")
	typingCtx, stopTyping := context.WithCancel(ctx)
	defer stopTyping()
	go b.keepTyping(typingCtx, chatID)

	force := strings.EqualFold(argument, "refresh")
	snapshot, err := b.quota.Snapshot(ctx, force)
	if err != nil {
		message := "❌ Gagal mengambil quota: " + err.Error()
		if loadingID != 0 && !b.editMessage(ctx, chatID, loadingID, message) {
			b.send(ctx, chatID, message)
		}
		return
	}
	filter := argument
	if force {
		filter = ""
	}
	text := renderSnapshot(snapshot, weeklyOnly, filter)
	parts := splitMessage(text)
	if loadingID != 0 && len(parts) > 0 && b.editMessage(ctx, chatID, loadingID, parts[0]) {
		for _, part := range parts[1:] {
			b.send(ctx, chatID, part)
		}
		return
	}
	for _, part := range parts {
		b.send(ctx, chatID, part)
	}
}

func (b *Bot) keepTyping(ctx context.Context, chatID int64) {
	b.sendChatAction(ctx, chatID)
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.sendChatAction(ctx, chatID)
		}
	}
}

func (b *Bot) allowRequest(userID int64) bool {
	b.lastRequestMu.Lock()
	defer b.lastRequestMu.Unlock()
	now := time.Now()
	if previous, ok := b.lastRequest[userID]; ok && now.Sub(previous) < b.minInterval {
		return false
	}
	b.lastRequest[userID] = now
	return true
}

func renderSnapshot(snapshot *antigravityquota.Snapshot, weeklyOnly bool, filter string) string {
	if snapshot == nil || len(snapshot.Accounts) == 0 {
		return "🚀 Antigravity Quota\n\nTidak ada koneksi Antigravity aktif di database Zyrouter."
	}
	var out strings.Builder
	filter = strings.ToLower(strings.TrimSpace(filter))
	visibleAccounts := 0
	out.WriteString("🚀 Antigravity Quota\n")
	out.WriteString("Update: ")
	out.WriteString(snapshot.FetchedAt.Local().Format("02 Jan 2006 15:04:05 MST"))
	out.WriteString("\n")
	for index, account := range snapshot.Accounts {
		if filter != "" && !strings.Contains(strings.ToLower(account.Email), filter) &&
			!strings.Contains(strings.ToLower(account.Name), filter) &&
			!strings.Contains(strings.ToLower(account.ConnectionID), filter) {
			continue
		}
		visibleAccounts++
		out.WriteString("\n")
		out.WriteString(fmt.Sprintf("Account %d: %s\n", index+1, safeAccountLabel(account)))
		if len(account.Windows) > 0 {
			for _, window := range account.Windows {
				if weeklyOnly && !strings.Contains(strings.ToLower(window.ID+" "+window.Label), "weekly") {
					continue
				}
				out.WriteString(fmt.Sprintf("%s: %.0f%% remaining", window.Label, window.RemainingPercentage))
				if window.ResetAt != "" {
					out.WriteString(" | reset ")
					out.WriteString(formatReset(window.ResetAt))
				}
				out.WriteString("\n")
			}
		}
		if !weeklyOnly && len(account.Models) > 0 {
			out.WriteString("\nPer-model:\n")
			for _, model := range account.Models {
				out.WriteString(fmt.Sprintf("• %s: %.0f%%", model.DisplayName, model.RemainingPercentage))
				if model.ResetAt != "" {
					out.WriteString(" | reset ")
					out.WriteString(formatReset(model.ResetAt))
				}
				out.WriteString("\n")
			}
		}
		if account.Error != "" {
			out.WriteString("⚠️ ")
			out.WriteString(account.Error)
			out.WriteString("\n")
		}
	}
	if visibleAccounts == 0 {
		return "🚀 Antigravity Quota\n\nTidak ada akun yang cocok dengan filter: " + filter
	}
	return out.String()
}

func safeAccountLabel(account antigravityquota.AccountQuota) string {
	label := account.Email
	if label == "" {
		label = account.Name
	}
	if label == "" {
		label = account.ConnectionID
	}
	return label
}

func formatReset(value string) string {
	reset, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	diff := time.Until(reset)
	if diff <= 0 {
		return "expired"
	}
	return "in " + formatDuration(diff)
}

func formatDuration(value time.Duration) string {
	minutes := int(value.Round(time.Minute) / time.Minute)
	if minutes < 1 {
		return "<1m"
	}
	days, minutes := minutes/1440, minutes%1440
	hours, minutes := minutes/60, minutes%60
	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 && len(parts) < 2 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	return strings.Join(parts, " ")
}

func parseCommand(text string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return "", ""
	}
	command := strings.TrimPrefix(strings.ToLower(parts[0]), "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	argument := ""
	if len(parts) > 1 {
		argument = strings.Join(parts[1:], " ")
	}
	return command, argument
}

func splitMessage(text string) []string {
	if len(text) <= messageLimit {
		return []string{text}
	}
	var result []string
	for len(text) > messageLimit {
		cut := strings.LastIndex(text[:messageLimit], "\n")
		if cut < 1 {
			cut = messageLimit
		}
		result = append(result, text[:cut])
		text = strings.TrimLeft(text[cut:], "\n")
	}
	if text != "" {
		result = append(result, text)
	}
	return result
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]update, error) {
	values := url.Values{}
	values.Set("offset", strconv.FormatInt(offset, 10))
	values.Set("timeout", strconv.Itoa(longPollTimeout))
	values.Set("allowed_updates", `["message"]`)
	response, err := call[[]update](b, ctx, "getUpdates", values)
	if err != nil {
		return nil, err
	}
	return response.Result, nil
}

func (b *Bot) send(ctx context.Context, chatID int64, text string) int64 {
	payload := map[string]string{"chat_id": strconv.FormatInt(chatID, 10), "text": text}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, telegramAPIBase+b.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return 0
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0
	}
	var result telegramResponse[sentMessage]
	if json.NewDecoder(response.Body).Decode(&result) != nil || !result.OK {
		return 0
	}
	return result.Result.MessageID
}

func (b *Bot) editMessage(ctx context.Context, chatID, messageID int64, text string) bool {
	payload := map[string]string{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"message_id": strconv.FormatInt(messageID, 10),
		"text":       text,
	}
	return b.postTelegramJSON(ctx, "editMessageText", payload)
}

func (b *Bot) sendChatAction(ctx context.Context, chatID int64) {
	payload := map[string]string{"chat_id": strconv.FormatInt(chatID, 10), "action": "typing"}
	_ = b.postTelegramJSON(ctx, "sendChatAction", payload)
}

func (b *Bot) postTelegramJSON(ctx context.Context, method string, payload interface{}) bool {
	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, telegramAPIBase+b.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return false
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var result telegramResponse[json.RawMessage]
	return json.NewDecoder(response.Body).Decode(&result) == nil && result.OK
}

func call[T any](b *Bot, ctx context.Context, method string, values url.Values) (*telegramResponse[T], error) {
	endpoint := telegramAPIBase + b.token + "/" + method
	if len(values) > 0 && method != "deleteWebhook" {
		endpoint += "?" + values.Encode()
	}
	var request *http.Request
	var err error
	if method == "deleteWebhook" {
		request, err = http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
		if err == nil {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		request, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	}
	if err != nil {
		return nil, err
	}
	response, err := b.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result telegramResponse[T]
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !result.OK {
		return nil, fmt.Errorf("Telegram API %s failed: HTTP %d: %s", method, response.StatusCode, result.Description)
	}
	return &result, nil
}

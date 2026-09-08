package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/log"
)

type BotService struct {
	repo       *db.Repo
	token      string
	username   string
	httpClient *http.Client
}

type telegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	IsBot     bool   `json:"is_bot"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type telegramMessage struct {
	MessageID int64         `json:"message_id"`
	From      *telegramUser `json:"from"`
	Chat      telegramChat  `json:"chat"`
	Text      string        `json:"text"`
}

type telegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type getMeResponse struct {
	OK     bool          `json:"ok"`
	Result *telegramUser `json:"result"`
}

type getUpdatesResponse struct {
	OK     bool             `json:"ok"`
	Result []telegramUpdate `json:"result"`
}

func NewBotService(repo *db.Repo, token, username string) *BotService {
	return &BotService{
		repo:       repo,
		token:      strings.TrimSpace(token),
		username:   strings.TrimPrefix(strings.TrimSpace(username), "@"),
		httpClient: &http.Client{Timeout: 35 * time.Second},
	}
}

// ExtractChallengeCode extracts the verification challenge ID/code from Telegram message text.
func ExtractChallengeCode(text string) string {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/start") {
		parts := strings.Fields(raw)
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[1])
		}
		return ""
	}
	if strings.HasPrefix(raw, "/verify") {
		parts := strings.Fields(raw)
		if len(parts) >= 2 {
			return strings.TrimSpace(parts[1])
		}
		return ""
	}
	// Direct code / UUID sent by user
	return raw
}

// Start begins background long polling for incoming Telegram updates.
func (b *BotService) Start(ctx context.Context) {
	if b.token == "" {
		log.Info("telegram", "TELEGRAM_BOT_TOKEN not configured; telegram bot service inactive")
		return
	}

	// Verify bot token and obtain username via getMe
	info, err := b.getMe(ctx)
	if err != nil {
		log.Warn("telegram", "failed to verify telegram bot token", "error", err)
	} else if info != nil {
		b.username = info.Username
		_ = os.Setenv("TELEGRAM_BOT_USERNAME", b.username)
		log.Info("telegram", "bot verified and active", "username", "@"+b.username, "botId", info.ID)
	}

	// Delete any active webhook so long polling getUpdates is allowed
	b.deleteWebhook(ctx)

	log.Info("telegram", "starting background long-polling for verification messages", "bot", "@"+b.username)
	var offset int64 = 0

	for {
		select {
		case <-ctx.Done():
			log.Info("telegram", "shutting down telegram bot listener")
			return
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn("telegram", "error polling getUpdates", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}

		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message != nil && u.Message.From != nil {
				b.handleMessage(ctx, u.Message)
			}
		}
	}
}

func (b *BotService) handleMessage(ctx context.Context, msg *telegramMessage) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	code := ExtractChallengeCode(text)
	if code == "" || strings.EqualFold(text, "/start") || strings.EqualFold(text, "/help") {
		b.sendMessage(ctx, msg.Chat.ID,
			"👋 <b>Halo!</b> Ini adalah bot verifikasi resmi <b>Zyrouter Gateway</b>.\n\n"+
				"Untuk menghubungkan akun Anda ke Client Dashboard:\n"+
				"1. Buka <b>Client Dashboard</b> Zyrouter di browser Anda.\n"+
				"2. Klik <b>Mulai Verifikasi</b> untuk mendapatkan kode rahasia.\n"+
				"3. Tekan link yang diberikan atau kirim <b>Kode Rahasia</b> Anda langsung ke chat ini.")
		return
	}

	telegramID := strconv.FormatInt(msg.From.ID, 10)
	name := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	if name == "" {
		name = msg.From.Username
	}

	user, err := b.repo.VerifyChallenge(code, telegramID, msg.From.Username, name, "user")
	if err != nil {
		log.Warn("telegram", "verification failed", "telegramID", telegramID, "error", err)
		b.sendMessage(ctx, msg.Chat.ID,
			fmt.Sprintf("❌ <b>Verifikasi Gagal</b>\n\n<i>%s</i>\n\nPastikan kode rahasia belum kedaluwarsa (berlaku 5 menit). Silakan buat kode baru dari dashboard Anda.", err.Error()))
		return
	}

	userTag := msg.From.Username
	if userTag != "" {
		userTag = "@" + userTag
	} else {
		userTag = name
	}

	log.Info("telegram", "challenge verified successfully", "userId", user.ID, "telegramID", telegramID, "tag", userTag)
	b.sendMessage(ctx, msg.Chat.ID,
		fmt.Sprintf("✅ <b>Verifikasi Berhasil!</b>\n\nAkun Telegram Anda (<b>%s</b>) telah berhasil di-binding ke akun Zyrouter Portal Anda.\n\nSilakan kembali ke browser Anda untuk mengakses Client Dashboard.", userTag))
}

func (b *BotService) getMe(ctx context.Context) (*telegramUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.telegram.org/bot"+b.token+"/getMe", nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result getMeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK || result.Result == nil {
		return nil, fmt.Errorf("telegram getMe returned not ok")
	}
	return result.Result, nil
}

func (b *BotService) deleteWebhook(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+b.token+"/deleteWebhook?drop_pending_updates=false", nil)
	if err != nil {
		return
	}
	resp, err := b.httpClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func (b *BotService) getUpdates(ctx context.Context, offset int64) ([]telegramUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=20&allowed_updates=[\"message\"]", b.token, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("telegram getUpdates HTTP %d: %s", resp.StatusCode, string(body))
	}

	var res getUpdatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	return res.Result, nil
}

func (b *BotService) sendMessage(ctx context.Context, chatID int64, text string) {
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.telegram.org/bot"+b.token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

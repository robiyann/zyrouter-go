package db

import (
	"encoding/json"
	"strings"

	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
)

// ProviderStrategy defines routing, fallback strategy, and proxy pool options for a specific provider.
type ProviderStrategy struct {
	FallbackStrategy      *string `json:"fallbackStrategy,omitempty"`      // "round-robin", "fallback", or null (default priority)
	StickyRoundRobinLimit int     `json:"stickyRoundRobinLimit,omitempty"` // default 1 or N requests
	ProxyPoolID           string  `json:"proxyPoolId,omitempty"`
	RotateStrategy        string  `json:"rotateStrategy,omitempty"` // "none", "round-robin", "random"
}

// SettingsData represents token saver and general settings stored in the settings table.
type SettingsData struct {
	RTKEnabled               bool                        `json:"rtkEnabled"`
	CavemanEnabled           bool                        `json:"cavemanEnabled"`
	CavemanLevel             string                      `json:"cavemanLevel"`
	PonytailEnabled          bool                        `json:"ponytailEnabled"`
	PonytailLevel            string                      `json:"ponytailLevel"`
	HeadroomUrl              string                      `json:"headroomUrl"`
	HeadroomCodeAware        bool                        `json:"headroomCodeAware"`
	HeadroomKompress         bool                        `json:"headroomKompress"`
	QuotaAutoRefreshInterval int                         `json:"quotaAutoRefreshInterval"`
	ProviderStrategies       map[string]ProviderStrategy `json:"providerStrategies,omitempty"`
	BackgroundImage          string                      `json:"backgroundImage,omitempty"`
	BackgroundOpacity        float64                     `json:"backgroundOpacity"`
	BackgroundBlur           int                         `json:"backgroundBlur"`
	GlassEnabled             bool                        `json:"glassEnabled"`
	GlassDepth               string                      `json:"glassDepth"`
	GlassSurfaceColor        string                      `json:"glassSurfaceColor"`
	GlassSurfaceOpacity      float64                     `json:"glassSurfaceOpacity"`
	BrandName                string                      `json:"brandName"`
	BrandSubtitle            string                      `json:"brandSubtitle"`
	BrandIcon                string                      `json:"brandIcon,omitempty"`
	Favicon                  string                      `json:"favicon,omitempty"`
	Password                 *string                     `json:"password,omitempty"`
	RequireLogin             *bool                       `json:"requireLogin,omitempty"`
}

// DefaultSettings returns fallback settings.
func DefaultSettings() *SettingsData {
	return &SettingsData{
		RTKEnabled:               true,
		CavemanEnabled:           false,
		CavemanLevel:             "full",
		PonytailEnabled:          false,
		PonytailLevel:            "full",
		HeadroomUrl:              "http://localhost:8787",
		HeadroomKompress:         true,
		QuotaAutoRefreshInterval: 60,
		BackgroundOpacity:        0.28,
		BackgroundBlur:           8,
		GlassEnabled:             true,
		GlassDepth:               "subtle",
		GlassSurfaceColor:        "#121216",
		BrandName:                "Zyrouter",
		BrandSubtitle:            "AI Routing Gateway",
	}
}

// GetSettings reads settings row id = 1 from SQLite settings table.
func (r *Repo) GetSettings() (*SettingsData, error) {
	var rawData string
	err := r.db.QueryRow(`SELECT data FROM settings WHERE id = 1`).Scan(&rawData)
	if err != nil {
		return DefaultSettings(), nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(rawData), &raw); err != nil {
		return DefaultSettings(), nil
	}

	s := DefaultSettings()
	if v, ok := raw["rtkEnabled"].(bool); ok {
		s.RTKEnabled = v
	}
	if v, ok := raw["cavemanEnabled"].(bool); ok {
		s.CavemanEnabled = v
	}
	if lvl := handlerutil.GetString(raw, "cavemanLevel"); lvl != "" {
		s.CavemanLevel = lvl
	}
	if v, ok := raw["ponytailEnabled"].(bool); ok {
		s.PonytailEnabled = v
	}
	if lvl := handlerutil.GetString(raw, "ponytailLevel"); lvl != "" {
		s.PonytailLevel = lvl
	}
	if v := handlerutil.GetString(raw, "headroomUrl"); v != "" {
		s.HeadroomUrl = v
	}
	if v, ok := raw["headroomCodeAware"].(bool); ok {
		s.HeadroomCodeAware = v
	}
	if v, ok := raw["headroomKompress"].(bool); ok {
		s.HeadroomKompress = v
	}
	if v, ok := raw["quotaAutoRefreshInterval"].(float64); ok {
		s.QuotaAutoRefreshInterval = int(v)
	}
	if v := getBoundedString(raw, "backgroundImage", 4<<20); v != "" {
		s.BackgroundImage = v
	}
	if v, ok := raw["backgroundOpacity"].(float64); ok {
		s.BackgroundOpacity = clampFloat(v, 0, 0.8)
	}
	if v, ok := raw["backgroundBlur"].(float64); ok {
		s.BackgroundBlur = clampInt(int(v), 0, 32)
	}
	if v, ok := raw["glassEnabled"].(bool); ok {
		s.GlassEnabled = v
	}
	if v := handlerutil.GetString(raw, "glassDepth"); v == "subtle" || v == "strong" {
		s.GlassDepth = v
	}
	if v := getBoundedString(raw, "glassSurfaceColor", 7); isHexColor(v) {
		s.GlassSurfaceColor = v
	}
	if v, ok := raw["glassSurfaceOpacity"].(float64); ok && v > 0 {
		s.GlassSurfaceOpacity = clampFloat(v, 0.08, 0.8)
	}
	if v := getBoundedString(raw, "brandName", 80); v != "" {
		s.BrandName = v
	}
	if v := getBoundedString(raw, "brandSubtitle", 120); v != "" {
		s.BrandSubtitle = v
	}
	if v := getBoundedString(raw, "brandIcon", 2<<20); v != "" {
		s.BrandIcon = v
	}
	if v := getBoundedString(raw, "favicon", 2<<20); v != "" {
		s.Favicon = v
	}
	if ps, ok := raw["providerStrategies"].(map[string]any); ok {
		s.ProviderStrategies = make(map[string]ProviderStrategy)
		for k, v := range ps {
			if vm, ok := v.(map[string]any); ok {
				strat := ProviderStrategy{
					ProxyPoolID:    handlerutil.GetString(vm, "proxyPoolId"),
					RotateStrategy: handlerutil.GetString(vm, "rotateStrategy"),
				}
				if fb := handlerutil.GetString(vm, "fallbackStrategy"); fb != "" {
					strat.FallbackStrategy = &fb
				}
				if sl, ok := vm["stickyRoundRobinLimit"].(float64); ok {
					strat.StickyRoundRobinLimit = int(sl)
				}
				s.ProviderStrategies[k] = strat
			}
		}
	}
	if pwd, ok := raw["password"].(string); ok && pwd != "" {
		s.Password = &pwd
	}
	if rl, ok := raw["requireLogin"].(bool); ok {
		s.RequireLogin = &rl
	}

	return s, nil
}

func getBoundedString(raw map[string]any, key string, maxBytes int) string {
	value := strings.TrimSpace(handlerutil.GetString(raw, key))
	if len(value) > maxBytes {
		return ""
	}
	return value
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func isHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, char := range value[1:] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

// UpdateSettingsData serializes and saves SettingsData to row id = 1.
func (r *Repo) UpdateSettingsData(s *SettingsData) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return r.SaveSettings(&models.Setting{ID: 1, Data: string(b)})
}

// SaveSettings writes or updates settings in the database.
func (r *Repo) SaveSettings(setting *models.Setting) error {
	_, err := r.db.Exec(`INSERT INTO settings (id, data) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data`, setting.Data)
	return err
}

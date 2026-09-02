package db

import (
	"encoding/json"

	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
)

// ProviderStrategy defines routing, fallback strategy, and proxy pool options for a specific provider.
type ProviderStrategy struct {
	FallbackStrategy      *string `json:"fallbackStrategy,omitempty"`      // "round-robin", "fallback", or null (default priority)
	StickyRoundRobinLimit int     `json:"stickyRoundRobinLimit,omitempty"` // default 1 or N requests
	ProxyPoolID           string  `json:"proxyPoolId,omitempty"`
	RotateStrategy        string  `json:"rotateStrategy,omitempty"`        // "none", "round-robin", "random"
}
// SettingsData represents token saver and general settings stored in the settings table.
type SettingsData struct {
	RTKEnabled         bool                        `json:"rtkEnabled"`
	CavemanEnabled     bool                        `json:"cavemanEnabled"`
	CavemanLevel       string                      `json:"cavemanLevel"`
	PonytailEnabled    bool                        `json:"ponytailEnabled"`
	PonytailLevel      string                      `json:"ponytailLevel"`
	HeadroomUrl        string                      `json:"headroomUrl"`
	HeadroomCodeAware  bool                        `json:"headroomCodeAware"`
	HeadroomKompress   bool                        `json:"headroomKompress"`
	ProviderStrategies map[string]ProviderStrategy `json:"providerStrategies,omitempty"`
	Password           *string                     `json:"password,omitempty"`
	RequireLogin       *bool                       `json:"requireLogin,omitempty"`
}
// DefaultSettings returns fallback settings.
func DefaultSettings() *SettingsData {
	return &SettingsData{
		RTKEnabled:       true,
		CavemanEnabled:   false,
		CavemanLevel:     "full",
		PonytailEnabled:  false,
		PonytailLevel:    "full",
		HeadroomUrl:      "http://localhost:8787",
		HeadroomKompress: true,
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

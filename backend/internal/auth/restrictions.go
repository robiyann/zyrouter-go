package auth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"zyrouter/backend/internal/models"
)

var (
	ErrKeyInactive        = errors.New("api key is disabled")
	ErrInvalidKeyPolicy   = errors.New("api key has invalid restrictions")
	ErrKeyExpired         = errors.New("api key has expired")
	ErrRateLimitExceeded  = errors.New("api key rate limit exceeded")
	ErrModelNotAllowed    = errors.New("api key is not permitted to access this model")
	ErrProviderNotAllowed = errors.New("api key is not permitted to access this provider")
)

// ValidateKeyPolicy checks whether the key is active and permitted to use the requested model and provider.
func ValidateKeyPolicy(key *models.APIKey, targetModel string, targetProvider string) error {
	if key == nil {
		return errors.New("missing api key")
	}
	if key.IsActive == 0 {
		return ErrKeyInactive
	}
	restrictions, err := key.ParseRestrictions()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidKeyPolicy, err)
	}
	if restrictions != nil && restrictions.ExpiresAt != nil && strings.TrimSpace(*restrictions.ExpiresAt) != "" {
		expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*restrictions.ExpiresAt))
		if err != nil {
			return fmt.Errorf("%w: invalid expiresAt", ErrInvalidKeyPolicy)
		}
		if !time.Now().Before(expiresAt) {
			return ErrKeyExpired
		}
	}

	// If a model is specified, validate model whitelist / wildcard prefixes
	if targetModel != "" {
		if !key.IsModelAllowed(targetModel) {
			return fmt.Errorf("%w: '%s'", ErrModelNotAllowed, targetModel)
		}
	}

	// If a provider connection is specified, validate provider whitelist
	if targetProvider != "" {
		if !key.IsProviderAllowed(targetProvider) {
			return fmt.Errorf("%w: '%s'", ErrProviderNotAllowed, targetProvider)
		}
	}

	return nil
}

// CleanModelName strips provider prefixes (e.g. "openai/gpt-4o" -> "gpt-4o") for normalization if needed.
func CleanModelName(model string) string {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return model
}

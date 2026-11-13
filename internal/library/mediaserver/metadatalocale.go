package mediaserver

import (
	"context"
	"net/http"
	"strings"
)

// systemConfigPath is the media server's server-wide configuration; both flavors answer it.
const systemConfigPath = "/System/Configuration"

// MetadataLocale is the media server's metadata language (such as "en" or "pt-BR") and its
// metadata country (such as "US"), as its settings name them; either is "" when unset.
func (c *Client) MetadataLocale(ctx context.Context) (language, country string, err error) {
	c, err = c.operation()
	if err != nil {
		return "", "", err
	}
	req, err := c.newRequest(ctx, http.MethodGet, systemConfigPath, nil)
	if err != nil {
		return "", "", err
	}
	c.flavor().applyTokenAuth(req, c.token(), c.deviceID)
	var cfg struct {
		Language string `json:"PreferredMetadataLanguage"`
		Country  string `json:"MetadataCountryCode"`
	}
	if err := c.do(req, &cfg); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(cfg.Language), strings.ToUpper(strings.TrimSpace(cfg.Country)), nil
}

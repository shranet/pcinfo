package network

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	// Faqat --public-ip bilan chaqiriladi: tashqi servisga so'rov ketadi.
	publicIPURL     = "https://api.ipify.org"
	publicIPTimeout = 6 * time.Second
	maxPublicIPBody = 64
)

// PublicIP iface orqali chiqib, tashqi IP manzilni so'raydi.
func PublicIP(ctx context.Context, iface string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, publicIPTimeout)
	defer cancel()
	client := &http.Client{
		Transport: &http.Transport{DialContext: BoundDialer(iface).DialContext},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, publicIPURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("public IP: %s", shortNetError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public IP: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPublicIPBody))
	if err != nil {
		return "", fmt.Errorf("public IP: %w", err)
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil {
		return "", fmt.Errorf("public IP: noto'g'ri javob")
	}
	return addr.String(), nil
}

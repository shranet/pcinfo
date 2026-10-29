// Package modem Huawei HiLink USB modemining (E3372) lokal web API'sidan
// signal, ulanish va trafik ma'lumotini o'qiydi hamda balans USSD'sini yuboradi.
//
// Modemda AT port yo'q, hamma narsa http://192.168.8.1 orqali. Har so'rovdan
// oldin sessiya olinadi: cookie va token Set-Cookie emas, XML ichida keladi.
package modem

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "http://192.168.8.1"

	pathSession = "/api/webserver/SesTokInfo"
	pathSignal  = "/api/device/signal"
	pathStatus  = "/api/monitoring/status"
	pathTraffic = "/api/monitoring/traffic-statistics"

	requestTimeout   = 5 * time.Second
	maxResponseBytes = 64 << 10

	headerCookie = "Cookie"
	headerToken  = "__RequestVerificationToken"

	// errCodeBadSession — cookie yoki token eskirgan; yangi sessiya kerak.
	errCodeBadSession = "125002"
)

// Client — modem API mijozi. Faqat shu PC ichidagi manzilga murojaat qiladi.
type Client struct {
	baseURL string
	http    *http.Client
}

// New baseURL bo'yicha mijoz yaratadi (odatda DefaultBaseURL).
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: requestTimeout,
			// Modem hech qachon boshqa joyga yo'naltirmasligi kerak.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// APIError — modem <error><code>…</code></error> qaytardi.
type APIError struct{ Code string }

func (e *APIError) Error() string { return "modem xatosi " + e.Code }

func isBadSession(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == errCodeBadSession
}

type session struct {
	cookie string
	token  string
}

func (c *Client) session(ctx context.Context) (session, error) {
	var resp struct {
		SesInfo string `xml:"SesInfo"`
		TokInfo string `xml:"TokInfo"`
	}
	if err := c.do(ctx, http.MethodGet, pathSession, session{}, nil, &resp); err != nil {
		return session{}, fmt.Errorf("sessiya: %w", err)
	}
	if resp.SesInfo == "" || resp.TokInfo == "" {
		return session{}, errors.New("sessiya: bo'sh javob")
	}
	return session{cookie: resp.SesInfo, token: resp.TokInfo}, nil
}

// do so'rov yuboradi va XML javobni out ga o'qiydi. Javob hajmi chegaralangan.
func (c *Client) do(ctx context.Context, method, path string, s session, body []byte, out any) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return err
	}
	if s.cookie != "" {
		req.Header.Set(headerCookie, s.cookie)
	}
	if body != nil {
		req.Header.Set(headerToken, s.token)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("modem javob bermadi")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return err
	}
	return decode(data, out)
}

func decode(data []byte, out any) error {
	var probe struct {
		XMLName xml.Name
		Code    string `xml:"code"`
	}
	if err := xml.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("XML: %w", err)
	}
	if probe.XMLName.Local == "error" {
		return &APIError{Code: strings.TrimSpace(probe.Code)}
	}
	if out == nil {
		return nil
	}
	if err := xml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("XML: %w", err)
	}
	return nil
}

// Signal — /api/device/signal (qiymatlar modem yozganidek: "-106dBm").
type Signal struct {
	RSSI string `xml:"rssi" json:"rssi"`
	RSRP string `xml:"rsrp" json:"rsrp"`
	RSRQ string `xml:"rsrq" json:"rsrq"`
	SINR string `xml:"sinr" json:"sinr"`
}

// Status — /api/monitoring/status.
type Status struct {
	ConnectionStatus     string `xml:"ConnectionStatus" json:"connectionStatus"`
	CurrentNetworkType   string `xml:"CurrentNetworkType" json:"currentNetworkType"`
	CurrentNetworkTypeEx string `xml:"CurrentNetworkTypeEx" json:"currentNetworkTypeEx"`
	SignalIcon           string `xml:"SignalIcon" json:"signalIcon"`
	MaxSignal            string `xml:"maxsignal" json:"maxSignal"`
}

// Traffic — /api/monitoring/traffic-statistics.
type Traffic struct {
	CurrentConnectTime  uint64 `xml:"CurrentConnectTime" json:"currentConnectTime"`
	CurrentUpload       uint64 `xml:"CurrentUpload" json:"currentUpload"`
	CurrentDownload     uint64 `xml:"CurrentDownload" json:"currentDownload"`
	CurrentDownloadRate uint64 `xml:"CurrentDownloadRate" json:"currentDownloadRate"`
	CurrentUploadRate   uint64 `xml:"CurrentUploadRate" json:"currentUploadRate"`
	TotalUpload         uint64 `xml:"TotalUpload" json:"totalUpload"`
	TotalDownload       uint64 `xml:"TotalDownload" json:"totalDownload"`
}

// Info — bir martada o'qiladigan arzon ma'lumotlar (USSD emas).
type Info struct {
	Signal  Signal  `json:"signal"`
	Status  Status  `json:"status"`
	Traffic Traffic `json:"traffic"`
}

// Info signal, holat va trafikni o'qiydi.
func (c *Client) Info(ctx context.Context) (Info, error) {
	s, err := c.session(ctx)
	if err != nil {
		return Info{}, err
	}
	var info Info
	for _, q := range []struct {
		path string
		out  any
	}{
		{pathSignal, &info.Signal},
		{pathStatus, &info.Status},
		{pathTraffic, &info.Traffic},
	} {
		if err := c.do(ctx, http.MethodGet, q.path, s, nil, q.out); err != nil {
			return Info{}, fmt.Errorf("%s: %w", q.path, err)
		}
	}
	return info, nil
}

// ConnectionText — ulanish holati kodini matnga aylantiradi.
func ConnectionText(code string) string {
	switch code {
	case "900":
		return "ulanmoqda"
	case "901":
		return "ulangan"
	case "902":
		return "uzilgan"
	case "903":
		return "uzilmoqda"
	case "":
		return "—"
	}
	return "holat " + code
}

// NetworkTypeText — CurrentNetworkType kodini matnga aylantiradi.
func NetworkTypeText(code string) string {
	switch code {
	case "0":
		return "xizmat yo'q"
	case "1", "2", "3":
		return "2G"
	case "4", "5", "6", "7", "9":
		return "3G"
	case "19", "101":
		return "4G LTE"
	case "":
		return "—"
	}
	return "tur " + code
}

// Quality — signal sifati.
type Quality int

const (
	QualityUnknown Quality = iota
	QualityPoor
	QualityFair
	QualityGood
)

// Chegaralar LTE uchun keng tarqalgan baholash bo'yicha.
const (
	rsrpGood = -90
	rsrpFair = -105
	sinrGood = 13
	sinrFair = 0
)

// RSRPQuality "-106dBm" kabi qiymatni baholaydi.
func RSRPQuality(v string) Quality { return grade(v, rsrpGood, rsrpFair) }

// SINRQuality "-2dB" kabi qiymatni baholaydi.
func SINRQuality(v string) Quality { return grade(v, sinrGood, sinrFair) }

func grade(v string, good, fair int) Quality {
	n, ok := ParseDB(v)
	switch {
	case !ok:
		return QualityUnknown
	case n >= good:
		return QualityGood
	case n >= fair:
		return QualityFair
	}
	return QualityPoor
}

// ParseDB "-106dBm", "-2dB", ">=-51dBm" dan butun sonni ajratadi.
func ParseDB(v string) (int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimLeft(v, "<>=")
	v = strings.TrimSuffix(strings.TrimSuffix(v, "m"), "dB")
	n, err := strconv.Atoi(v)
	return n, err == nil
}

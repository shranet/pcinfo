package modem

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// BalanceCode — balans va MB qoldig'ini so'raydigan USSD.
	BalanceCode = "*107#"

	// MinInterval — operator cheklamasligi uchun USSD'lar orasidagi eng kam vaqt.
	MinInterval = 5 * time.Minute

	pathUSSDSend   = "/api/ussd/send"
	pathUSSDStatus = "/api/ussd/status"
	pathUSSDGet    = "/api/ussd/get"

	ussdPollInterval = 2 * time.Second
	ussdMaxWait      = 40 * time.Second

	ussdStatusReady = "0"
	ussdSendOK      = "OK"

	// ussdLockName — Linux abstract unix socket: diskka hech narsa yozmasdan
	// bir vaqtda faqat bitta USSD bo'lishini ta'minlaydi (oldingisini bekor qilmaslik uchun).
	ussdLockName = "@pcinfo-ussd-lock"
)

var (
	ussdCodePattern = regexp.MustCompile(`^[0-9*#]{1,20}$`)

	balancePattern = regexp.MustCompile(`(?i)balans\s*:\s*(-?[0-9]+(?:[.,][0-9]+)?)`)
	minutesPattern = regexp.MustCompile(`(?i)([0-9]+)\s*MIN\b`)
	mbPattern      = regexp.MustCompile(`(?i)([0-9]+)\s*MB\b`)
	smsPattern     = regexp.MustCompile(`(?i)([0-9]+)\s*SMS\b`)
)

// ErrBusy — boshqa pcinfo jarayoni hozir USSD yuboryapti.
var ErrBusy = errors.New("boshqa pcinfo USSD yuboryapti")

// Stage — USSD bosqichi (UI'da ko'rsatiladi).
type Stage int

const (
	StageSession Stage = iota
	StageSent
	StageWaiting
)

// Progress — USSD jarayonidagi holat.
type Progress struct {
	Stage   Stage
	Waited  time.Duration
	MaxWait time.Duration
}

// Balance — *107# javobi.
type Balance struct {
	Raw     string `json:"raw"`
	Balance string `json:"balance"`
	Minutes *int64 `json:"minutes,omitempty"`
	MB      *int64 `json:"mb,omitempty"`
	SMS     *int64 `json:"sms,omitempty"`
}

// USSD code ni yuboradi, javobni kutadi va qaytaradi. report har bosqichda
// chaqiriladi (nil bo'lishi mumkin). Bir vaqtda faqat bitta USSD.
func (c *Client) USSD(ctx context.Context, code string, report func(Progress)) (Balance, error) {
	if !ussdCodePattern.MatchString(code) {
		return Balance{}, fmt.Errorf("noto'g'ri USSD kod %q", code)
	}
	lock, err := net.Listen("unix", ussdLockName)
	if err != nil {
		return Balance{}, ErrBusy
	}
	defer lock.Close()

	notify := func(p Progress) {
		if report != nil {
			report(p)
		}
	}
	notify(Progress{Stage: StageSession})
	s, err := c.sendUSSD(ctx, code)
	if err != nil {
		return Balance{}, err
	}
	notify(Progress{Stage: StageSent})

	if err := c.waitUSSD(ctx, s, notify); err != nil {
		return Balance{}, err
	}
	var resp struct {
		Content string `xml:"content"`
	}
	if err := c.do(ctx, http.MethodGet, pathUSSDGet, s, nil, &resp); err != nil {
		return Balance{}, fmt.Errorf("javobni o'qish: %w", err)
	}
	return ParseBalance(resp.Content), nil
}

// sendUSSD yuboradi; sessiya eskirgan bo'lsa bir marta yangisi bilan qaytaradi.
func (c *Client) sendUSSD(ctx context.Context, code string) (session, error) {
	body, err := ussdRequest(code)
	if err != nil {
		return session{}, err
	}
	const attempts = 2
	var lastErr error
	for range attempts {
		s, err := c.session(ctx)
		if err != nil {
			return session{}, err
		}
		var resp string
		err = c.do(ctx, http.MethodPost, pathUSSDSend, s, body, &resp)
		switch {
		case err == nil && strings.TrimSpace(resp) == ussdSendOK:
			return s, nil
		case err == nil:
			return session{}, fmt.Errorf("yuborish: kutilmagan javob")
		case !isBadSession(err):
			return session{}, fmt.Errorf("yuborish: %w", err)
		}
		lastErr = err
	}
	return session{}, fmt.Errorf("yuborish: %w", lastErr)
}

func ussdRequest(code string) ([]byte, error) {
	type request struct {
		XMLName  xml.Name `xml:"request"`
		Content  string   `xml:"content"`
		CodeType string   `xml:"codeType"`
		Timeout  string   `xml:"timeout"`
	}
	body, err := xml.Marshal(request{Content: code, CodeType: "CodeType"})
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}

func (c *Client) waitUSSD(ctx context.Context, s session, notify func(Progress)) error {
	start := time.Now()
	ticker := time.NewTicker(ussdPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		waited := time.Since(start)
		var resp struct {
			Result string `xml:"result"`
		}
		if err := c.do(ctx, http.MethodGet, pathUSSDStatus, s, nil, &resp); err != nil {
			return fmt.Errorf("holat: %w", err)
		}
		if strings.TrimSpace(resp.Result) == ussdStatusReady {
			return nil
		}
		if waited >= ussdMaxWait {
			return fmt.Errorf("javob %s ichida kelmadi", ussdMaxWait)
		}
		notify(Progress{Stage: StageWaiting, Waited: waited, MaxWait: ussdMaxWait})
	}
}

// ParseBalance operator matnidan balans, daqiqa, MB va SMS ni ajratadi.
// Format o'zgarishi mumkin, shuning uchun xom matn ham saqlanadi.
func ParseBalance(raw string) Balance {
	b := Balance{Raw: strings.TrimSpace(raw)}
	if m := balancePattern.FindStringSubmatch(raw); m != nil {
		b.Balance = m[1]
	}
	b.Minutes = firstInt(minutesPattern, raw)
	b.MB = firstInt(mbPattern, raw)
	b.SMS = firstInt(smsPattern, raw)
	return b
}

func firstInt(re *regexp.Regexp, s string) *int64 {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

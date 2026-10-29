package modem

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const sessionXML = `<?xml version="1.0" encoding="UTF-8"?><response><SesInfo>SessionID=abc</SesInfo><TokInfo>tok1</TokInfo></response>`

// fakeModem — HiLink API'ning minimal soxta nusxasi.
type fakeModem struct {
	mu          sync.Mutex
	badSessions int // shuncha send so'rovi 125002 bilan qaytadi
	pendingPoll int // shuncha status so'rovi "1" (kutilmoqda) qaytaradi
	sends       int
	content     string
	lastBody    string
	lastToken   string
}

func (f *fakeModem) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case pathSession:
		io.WriteString(w, sessionXML)
	case pathSignal:
		io.WriteString(w, `<response><rssi>-73dBm</rssi><rsrp>-106dBm</rsrp><rsrq>-12dB</rsrq><sinr>-2dB</sinr></response>`)
	case pathStatus:
		io.WriteString(w, `<response><ConnectionStatus>901</ConnectionStatus><CurrentNetworkType>19</CurrentNetworkType><SignalIcon>4</SignalIcon></response>`)
	case pathTraffic:
		io.WriteString(w, `<response><CurrentConnectTime>120</CurrentConnectTime><CurrentDownloadRate>2048</CurrentDownloadRate><TotalDownload>1000000</TotalDownload></response>`)
	case pathUSSDSend:
		if r.Method != http.MethodPost || r.Header.Get(headerCookie) != "SessionID=abc" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.sends++
		body, _ := io.ReadAll(r.Body)
		f.lastBody = string(body)
		f.lastToken = r.Header.Get(headerToken)
		if f.badSessions > 0 {
			f.badSessions--
			io.WriteString(w, `<?xml version="1.0"?><error><code>125002</code><message></message></error>`)
			return
		}
		io.WriteString(w, `<response>OK</response>`)
	case pathUSSDStatus:
		result := "0"
		if f.pendingPoll > 0 {
			f.pendingPoll--
			result = "1"
		}
		io.WriteString(w, `<response><result>`+result+`</result></response>`)
	case pathUSSDGet:
		io.WriteString(w, `<response><content>`+f.content+`</content></response>`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestInfo(t *testing.T) {
	srv := httptest.NewServer(&fakeModem{})
	defer srv.Close()

	info, err := New(srv.URL).Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Signal.RSRP != "-106dBm" || info.Status.ConnectionStatus != "901" || info.Traffic.CurrentDownloadRate != 2048 {
		t.Errorf("info = %+v", info)
	}
}

func TestInfoUnreachable(t *testing.T) {
	srv := httptest.NewServer(&fakeModem{})
	url := srv.URL
	srv.Close()
	if _, err := New(url).Info(context.Background()); err == nil {
		t.Error("xato kutilgan edi")
	}
}

func TestDecodeAPIError(t *testing.T) {
	err := decode([]byte(`<error><code>100003</code></error>`), nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != "100003" {
		t.Errorf("err = %v", err)
	}
	if err := decode([]byte(`not xml`), nil); err == nil {
		t.Error("buzilgan XML xato berishi kerak")
	}
}

func TestUSSD(t *testing.T) {
	fm := &fakeModem{
		badSessions: 1,
		pendingPoll: 1,
		content:     "Balans: 5000, Dostupno: 5000, Ostatok: 0 MIN; 304519 MB; 101 SMS &#x2F; &apos;ok&apos;",
	}
	srv := httptest.NewServer(fm)
	defer srv.Close()

	var stages []Stage
	bal, err := New(srv.URL).USSD(context.Background(), BalanceCode, func(p Progress) {
		stages = append(stages, p.Stage)
	})
	if err != nil {
		t.Fatal(err)
	}
	if fm.sends != 2 {
		t.Errorf("125002 dan keyin qayta yuborilishi kerak edi, sends=%d", fm.sends)
	}
	if !strings.Contains(fm.lastBody, "<content>*107#</content>") || fm.lastToken != "tok1" {
		t.Errorf("so'rov tanasi/token: %q %q", fm.lastBody, fm.lastToken)
	}
	if bal.Balance != "5000" || *bal.MB != 304519 || *bal.Minutes != 0 || *bal.SMS != 101 {
		t.Errorf("balans = %+v", bal)
	}
	if !strings.Contains(bal.Raw, "/ 'ok'") {
		t.Errorf("XML entity'lar decode qilinishi kerak: %q", bal.Raw)
	}
	want := []Stage{StageSession, StageSent, StageWaiting}
	if len(stages) != len(want) {
		t.Fatalf("bosqichlar = %v", stages)
	}
}

func TestUSSDRejectsBadCode(t *testing.T) {
	for _, code := range []string{"", "*107#<x>", "ls; rm", strings.Repeat("1", 21)} {
		if _, err := New("http://127.0.0.1:1").USSD(context.Background(), code, nil); err == nil {
			t.Errorf("%q rad etilishi kerak edi", code)
		}
	}
}

func TestUSSDBusy(t *testing.T) {
	fm := &fakeModem{pendingPoll: 100, content: "x"}
	srv := httptest.NewServer(fm)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		New(srv.URL).USSD(ctx, BalanceCode, func(p Progress) {
			if p.Stage == StageSent {
				close(started)
			}
		})
	}()
	<-started
	if _, err := New(srv.URL).USSD(context.Background(), BalanceCode, nil); !errors.Is(err, ErrBusy) {
		t.Errorf("ikkinchi USSD ErrBusy qaytarishi kerak, err = %v", err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("birinchi USSD bekor qilinmadi")
	}
}

func TestParseBalance(t *testing.T) {
	b := ParseBalance("Hisobingizda mablag' yo'q")
	if b.Balance != "" || b.MB != nil || b.Minutes != nil || b.SMS != nil {
		t.Errorf("topilmagan maydonlar bo'sh bo'lishi kerak: %+v", b)
	}
	b = ParseBalance("balans: -120.50 ; 12 mb")
	if b.Balance != "-120.50" || b.MB == nil || *b.MB != 12 {
		t.Errorf("b = %+v", b)
	}
}

func TestQuality(t *testing.T) {
	tests := []struct {
		got, want Quality
	}{
		{RSRPQuality("-80dBm"), QualityGood},
		{RSRPQuality("-100dBm"), QualityFair},
		{RSRPQuality("-106dBm"), QualityPoor},
		{RSRPQuality(""), QualityUnknown},
		{SINRQuality("-2dB"), QualityPoor},
		{SINRQuality("5dB"), QualityFair},
		{SINRQuality(">=20dB"), QualityGood},
	}
	for i, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("#%d: %v, want %v", i, tt.got, tt.want)
		}
	}
}

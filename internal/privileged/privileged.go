// Package privileged claude tomonidagi ma'lumotni oladi: TUI claude bo'lmasa
// `sudo -n -u claude /usr/local/bin/pcinfo collect --json` orqali (D-002, D-003).
//
// Kelgan JSON ishonchsiz: hajmi chegaralanadi, schema tekshiriladi va barcha
// satrlar terminalga chiqishdan oldin tozalanadi.
package privileged

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shranet/pcinfo/internal/safetext"
	"github.com/shranet/pcinfo/internal/snapshot"
)

const (
	ClaudeUser      = "claude"
	InstalledBinary = "/usr/local/bin/pcinfo"
	sudoBinary      = "/usr/bin/sudo"

	fetchTimeout   = 40 * time.Second
	sudoWaitDelay  = 3 * time.Second
	maxOutputBytes = 16 << 20
	maxStderrBytes = 2 << 10

	// groupOtherWrite — guruh yoki boshqalar yoza oladigan bitlar.
	groupOtherWrite = 0o022
)

// Fetcher claude tomonini oladi.
type Fetcher struct {
	isClaude bool
	binary   string
}

// NewFetcher joriy foydalanuvchi claude ekanini aniqlaydi.
func NewFetcher() *Fetcher {
	isClaude := false
	if u, err := user.Lookup(ClaudeUser); err == nil {
		isClaude = u.Uid == strconv.Itoa(os.Getuid())
	}
	return &Fetcher{isClaude: isClaude, binary: InstalledBinary}
}

// IsClaude — TUI claude nomidan ishlayaptimi (unda sudo kerak emas).
func (f *Fetcher) IsClaude() bool { return f.isClaude }

// Fetch claude tomonini qaytaradi.
func (f *Fetcher) Fetch(ctx context.Context) (snapshot.ClaudeSide, error) {
	if f.isClaude {
		s := snapshot.CollectClaudeSide(ctx)
		safetext.CleanAll(&s)
		return s, nil
	}
	if err := CheckBinary(f.binary); err != nil {
		return snapshot.ClaudeSide{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, sudoBinary, "-n", "-u", ClaudeUser, "--", f.binary, "collect", "--json")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	// SIGTERM'ni sudo bola jarayonga uzatadi. WaitDelay'siz claude o'z collect jarayonini
	// to'xtatib (SIGSTOP) yoki stdout'ni ochiq ushlab, Run()'ni abadiy kuttirib qo'yishi mumkin edi.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = sudoWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &capWriter{buf: &stdout, limit: maxOutputBytes}
	cmd.Stderr = &capWriter{buf: &stderr, limit: maxStderrBytes}
	if err := cmd.Run(); err != nil {
		return snapshot.ClaudeSide{}, sudoError(err, stderr.String())
	}
	return Decode(&stdout)
}

// Decode `collect --json` chiqishini o'qiydi va tozalaydi.
func Decode(r io.Reader) (snapshot.ClaudeSide, error) {
	var s snapshot.ClaudeSide
	dec := json.NewDecoder(io.LimitReader(r, maxOutputBytes))
	if err := dec.Decode(&s); err != nil {
		return snapshot.ClaudeSide{}, fmt.Errorf("claude javobi o'qilmadi: %w", err)
	}
	if s.Schema != snapshot.Schema {
		return snapshot.ClaudeSide{}, fmt.Errorf("%s versiyasi mos emas (schema %d, kerak %d): qayta o'rnating", InstalledBinary, s.Schema, snapshot.Schema)
	}
	safetext.CleanAll(&s)
	return s, nil
}

func sudoError(err error, stderr string) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("claude ma'lumoti: timeout")
	case strings.Contains(stderr, "password is required"), strings.Contains(stderr, "not allowed"):
		return errors.New("sudo ruxsati yo'q: README dagi o'rnatishni bajaring")
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		return errors.New("sudo topilmadi")
	}
	return fmt.Errorf("claude ma'lumoti olinmadi: %s", safetext.Clean(firstLine(stderr)))
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// CheckBinary sudo bilan ishga tushiriladigan faylni tekshiradi: oddiy fayl,
// egasi root, guruh/boshqalar yoza olmaydi; papkasi ham shunday. Aks holda
// claude faylni almashtirib, shranet nomidan kod ishga tushirishi mumkin edi.
func CheckBinary(path string) error {
	for _, p := range []string{path, filepath.Dir(path)} {
		fi, err := os.Lstat(p)
		if err != nil {
			return fmt.Errorf("%s topilmadi: README dagi o'rnatishni bajaring", p)
		}
		if p == path && !fi.Mode().IsRegular() {
			return fmt.Errorf("%s oddiy fayl emas (symlink?) — xavfsizlik uchun ishlatilmaydi", p)
		}
		if err := ownedByRootNotWritable(fi); err != nil {
			return fmt.Errorf("%s: %w — xavfsizlik uchun ishlatilmaydi", p, err)
		}
	}
	return nil
}

func ownedByRootNotWritable(fi fs.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("egasi aniqlanmadi")
	}
	if st.Uid != 0 {
		return errors.New("egasi root emas")
	}
	if fi.Mode().Perm()&groupOtherWrite != 0 {
		return errors.New("boshqalar yoza oladi")
	}
	return nil
}

// SelfCheck ishlayotgan binary'ni tekshiradi: uni boshqa foydalanuvchi
// (masalan, claude) o'zgartira olsa, TUI shranet nomidan begona kodni ishga tushiradi.
func SelfCheck() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	var warnings []string
	for _, p := range []string{exe, filepath.Dir(exe)} {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		owner := int(st.Uid)
		if owner != 0 && owner != os.Getuid() {
			warnings = append(warnings, fmt.Sprintf("%s egasi boshqa foydalanuvchi (uid %d)", p, owner))
		}
		if fi.Mode().Perm()&groupOtherWrite != 0 && fi.Mode()&fs.ModeSticky == 0 {
			warnings = append(warnings, fmt.Sprintf("%s ga boshqalar yoza oladi", p))
		}
	}
	return warnings
}

// capWriter limitdan oshgan chiqishni tashlaydi; limit oshsa xato beradi,
// shunda chala JSON jim qabul qilinmaydi.
type capWriter struct {
	buf   *bytes.Buffer
	limit int
}

var errOutputTooLarge = errors.New("chiqish juda katta")

func (c *capWriter) Write(p []byte) (int, error) {
	if c.buf.Len()+len(p) > c.limit {
		return 0, errOutputTooLarge
	}
	return c.buf.Write(p)
}

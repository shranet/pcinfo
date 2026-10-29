package privileged

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shranet/pcinfo/internal/snapshot"
)

func TestDecode(t *testing.T) {
	in := `{"schema":1,"user":"claude\u001b]0;x\u0007","projects":{"projects":[{"workspace":{"name":"a\u001b[31mb"}}]}}`
	s, err := Decode(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if s.User != "claude]0;x" || s.Projects.Projects[0].Workspace.Name != "a[31mb" {
		t.Errorf("escape'lar tozalanmadi: %q %q", s.User, s.Projects.Projects[0].Workspace.Name)
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := map[string]string{
		"buzilgan":      `{"schema":`,
		"boshqa schema": `{"schema":999}`,
		"schema yo'q":   `{}`,
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(in)); err == nil {
				t.Error("xato kutilgan edi")
			}
		})
	}
	if snapshot.Schema != 1 {
		t.Log("schema o'zgardi — testdagi JSON'larni yangilang")
	}
}

func TestCheckBinary(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "pcinfo")
	if err := os.WriteFile(own, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(own, link); err != nil {
		t.Fatal(err)
	}
	isRoot := os.Getuid() == 0
	for _, p := range []string{own, link, filepath.Join(dir, "yo'q")} {
		err := CheckBinary(p)
		// root bo'lmagan foydalanuvchi yaratgan fayl doim rad etiladi;
		// symlink va yo'q fayl esa root uchun ham rad etiladi.
		if err == nil && (!isRoot || p != own) {
			t.Errorf("%s rad etilishi kerak edi", p)
		}
	}
	if isRoot {
		if err := os.Chmod(own, 0o777); err != nil {
			t.Fatal(err)
		}
		if err := CheckBinary(own); err == nil {
			t.Error("hamma yoza oladigan fayl rad etilishi kerak")
		}
	}
}

func TestCapWriter(t *testing.T) {
	var b bytes.Buffer
	w := &capWriter{buf: &b, limit: 3}
	if _, err := w.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("cd")); !errors.Is(err, errOutputTooLarge) {
		t.Errorf("err = %v", err)
	}
}

func TestSudoError(t *testing.T) {
	err := sudoError(errors.New("exit 1"), "sudo: a password is required\n")
	if !strings.Contains(err.Error(), "sudo ruxsati yo'q") {
		t.Errorf("err = %v", err)
	}
	err = sudoError(errors.New("exit 1"), "boom\x1b[2J\nikkinchi")
	if strings.Contains(err.Error(), "\x1b") || strings.Contains(err.Error(), "ikkinchi") {
		t.Errorf("stderr tozalanmadi: %q", err)
	}
}

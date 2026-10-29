package safetext

import "testing"

func TestClean(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"bo'sh", "", ""},
		{"oddiy", "lens-web-1", "lens-web-1"},
		{"unicode saqlanadi", "Тест ✓ o'zbek", "Тест ✓ o'zbek"},
		{"CSI olib tashlanadi", "a\x1b[31mb", "a[31mb"},
		{"OSC sarlavha", "\x1b]2;pwned\x07x", "]2;pwnedx"},
		{"C1 CSI", "a\u009bb", "ab"},
		{"DEL", "a\x7fb", "ab"},
		{"tab va yangi qator", "a\tb\nc\rd", "a b c d"},
		{"bidi override", "abc‮def", "abcdef"},
		{"noto'g'ri UTF-8", "a\xffb", "a?b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Clean(tt.in); got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanAll(t *testing.T) {
	type inner struct {
		Name    string
		private string
	}
	type outer struct {
		Title string
		Items []inner
		Ptr   *inner
		Tags  []string
		Count int
	}
	v := outer{
		Title: "x\x1b[2Jy",
		Items: []inner{{Name: "\x1b]0;t\x07", private: "\x1b"}},
		Ptr:   &inner{Name: "a\u009bb"},
		Tags:  []string{"ok", "\x07bell"},
		Count: 3,
	}
	CleanAll(&v)

	if v.Title != "x[2Jy" {
		t.Errorf("Title = %q", v.Title)
	}
	if v.Items[0].Name != "]0;t" {
		t.Errorf("Items[0].Name = %q", v.Items[0].Name)
	}
	if v.Items[0].private != "\x1b" {
		t.Errorf("eksport qilinmagan maydonga tegilmasligi kerak")
	}
	if v.Ptr.Name != "ab" {
		t.Errorf("Ptr.Name = %q", v.Ptr.Name)
	}
	if v.Tags[1] != "bell" {
		t.Errorf("Tags[1] = %q", v.Tags[1])
	}
}

func TestCleanAllNilPointer(t *testing.T) {
	var p *struct{ Name string }
	CleanAll(p)
	CleanAll(nil)
}

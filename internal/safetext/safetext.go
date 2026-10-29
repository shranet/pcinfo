// Package safetext tashqi manbadan kelgan matnni terminalga chiqarishga tayyorlaydi.
//
// Jarayon nomi, konteyner nomi, git branch, modem javobi kabi satrlar ichida
// ESC ketma-ketliklari bo'lishi mumkin. Ular tozalanmasa, boshqa foydalanuvchi
// (masalan, claude) shranet terminalini boshqarib qo'yishi mumkin: sarlavhani
// o'zgartirish, ekranni soxtalashtirish, ba'zi terminallarda buyruq yozish.
package safetext

import (
	"reflect"
	"strings"
	"unicode/utf8"
)

// replacement — noto'g'ri UTF-8 va tab/yangi qator o'rniga qo'yiladigan belgi.
const (
	invalidReplacement    = '?'
	whitespaceReplacement = ' '
)

// Clean boshqaruv belgilarini (C0, DEL, C1) va matn yo'nalishini almashtiruvchi
// Unicode belgilarni olib tashlaydi. Tab va yangi qator bo'sh joyga aylanadi.
func Clean(s string) string {
	if isClean(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i, w := 0, 0; i < len(s); i += w {
		r, size := utf8.DecodeRuneInString(s[i:])
		w = size
		switch {
		case r == utf8.RuneError && size <= 1:
			b.WriteRune(invalidReplacement)
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(whitespaceReplacement)
		case isUnsafe(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isClean(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if isUnsafe(r) {
			return false
		}
	}
	return true
}

func isUnsafe(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	// Bidi boshqaruv belgilari: ko'rinadigan matnni teskari tartibda ko'rsatib aldaydi.
	case r == 0x061c, r == 0x200e, r == 0x200f,
		r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// CleanAll struct, slice va pointer'lar ichidagi barcha eksport qilingan
// satr maydonlarini joyida tozalaydi. v — pointer bo'lishi kerak.
// Map'lar aylanib chiqilmaydi: kontrakt turlarida map ishlatilmaydi.
func CleanAll(v any) {
	cleanValue(reflect.ValueOf(v))
}

func cleanValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			cleanValue(v.Elem())
		}
	case reflect.Struct:
		t := v.Type()
		for i := range v.NumField() {
			if t.Field(i).IsExported() {
				cleanValue(v.Field(i))
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			cleanValue(v.Index(i))
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(Clean(v.String()))
		}
	}
}

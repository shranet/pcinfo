# pcinfo — kod yozish qoidalari

pcinfo — shu PC haqida htop'ga o'xshash terminal dasturi (TUI): tizim, jarayonlar,
Docker, `/srv/data/claude/projects` dagi proyektlar va ularning preview'lari,
internet liniyalari (optika / USB modem) va modem balansi.
Vazifa va bosqichlar: [docs/spec.md](docs/spec.md). Qarorlar: [docs/decisions.md](docs/decisions.md).

Bu fayl umumiy `code-patterns` skill'ining **shu proyektga moslangan** varianti.
Proyektda frontend (React), HTTP server, ma'lumotlar bazasi yo'q, shuning uchun
skill'ning o'sha bo'limlari bu yerda qo'llanmaydi. Qolgan tamoyillar shu yerda
TUI va tizim ma'lumotini yig'ishga moslab yozilgan. Global `~/.claude/CLAUDE.md`
qoidalari (Docker, git push) bundan ustun.

## 1. Asosiy tamoyillar

- **O'qiladigan kod birinchi o'rinda.** `cmd/pcinfo/main.go` dan boshlab o'qigan
  dasturchi tizimni tushunsin. Aqlli emas, ravshan kod.
- **Bitta mas'uliyat.** Har paket, fayl, tur, funksiya bitta ishni bajaradi;
  nomi shu ishni aytadi.
- **Bir yo'nalishli bog'liqlik:** `cmd` → `tui` → `snapshot` → `collect/*`.
  Kollektorlar `tui` ni bilmaydi; kollektor paketlari bir-birini import qilmaydi
  (ularni `snapshot` yig'adi). Aylana import va yashirin global holat yo'q.
- **Erta abstraksiya yo'q.** Bir xil mantiq uchinchi marta paydo bo'lganda
  umumlashtiriladi. Interfeys faqat iste'molchi tomonida va kerak bo'lganda
  (masalan, testda soxta manba berish uchun).
- **Sehrli qiymat yo'q.** Interval, timeout, chegara, yo'l, IP, rang —
  nomlangan konstanta (paket boshida) yoki mavzu tokeni.
- **Kichik diff.** So'ralmagan joy o'zgartirilmaydi.
- **Mavjud uslubga moslash:** nomlash, izoh zichligi, xato qaytarish usuli.

## 2. Stek

- **Go** (Docker image `golang:1.26.10`), statik binary: `CGO_ENABLED=0`.
- Ishlatilayotgan kutubxonalar (D-001, D-001a):
  - `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2` (va ularning `x/ansi`) — TUI;
  - `github.com/shirou/gopsutil/v4` — tizim ma'lumotlari.
- Docker API — standart `net/http`, faqat GET. Sichqoncha zonalari, jadval, sparkline —
  `internal/tui/widgets.go`.
- Boshqa har qanday kutubxona faqat asoslab va foydalanuvchidan so'rab qo'shiladi.
  Avval standart kutubxona: log `log/slog`, testlar `testing` (assert kutubxonasisiz).
  Linux ma'lumotini `/proc` va `/sys` dan o'qish kutubxonadan afzal, agar u
  20–30 qatorda ravshan yozilsa.

## 3. Tuzilma

```
cmd/pcinfo/            kirish nuqtasi: flaglar, rejim tanlash (tui | collect), bog'liqliklarni yig'ish
internal/collect/<x>/  kollektorlar, har biri bitta domen: system, process, docker,
                       projects, network, modem. Natija — oddiy struct, UI'siz.
internal/snapshot/     kollektorlarni yig'adi; claude tomoni bilan JSON kontrakti
internal/privileged/   `sudo -n -u claude -- /usr/local/bin/pcinfo collect --json` ni chaqirish va javobni tekshirish
internal/tui/          Bubble Tea modeli, tablar (har tab o'z faylida/paketida)
internal/tui/theme/    ranglar va stillar — yagona manba
docs/                  spec.md, decisions.md
```

- `main` faqat flaglarni o'qiydi, bog'liqliklarni yig'adi va rejimni ishga
  tushiradi. Mantiq u yerda bo'lmaydi.
- Bog'liqliklar konstruktor orqali beriladi. Paket darajasidagi o'zgaruvchan
  holat yo'q.
- `collect --json` chiqishi — **kontrakt**: `snapshot` paketidagi turlar,
  `schema` versiya maydoni bilan. Uni o'zgartirish ikki tomonni bir commit'da
  yangilashni anglatadi.

## 4. TUI qoidalari (Bubble Tea)

- **`Update` va `View` hech qachon bloklamaydi.** Fayl o'qish, `exec`, tarmoq,
  Docker API — faqat `tea.Cmd` ichida; natija xabar (`Msg`) bo'lib qaytadi.
- `View` — sof funksiya: faqat model holatidan matn yasaydi, yon ta'sirsiz.
- Har yangilash davri (tick) nomlangan konstanta; sekin manbalar (Docker, git,
  claude tomoni) tez manbalardan (CPU, tarmoq) alohida davrda.
- **Ranglar faqat `theme` tokenlari orqali.** Kodda `lipgloss.Color("#…")` yo'q;
  semantik nomlar (`OK`, `Warn`, `Err`, `Muted`, `Accent`). `NO_COLOR` hurmat qilinadi.
- Har panel to'rt holatni ko'rsatadi: **yuklanmoqda, bo'sh, xato, ma'lumot**.
  Bitta manba xatosi butun dasturni yiqitmaydi — faqat o'sha panel xato ko'rsatadi.
- Har amal ham **klaviatura**, ham **sichqoncha** bilan bajariladi; tugmalar
  ekranning pastida ko'rsatiladi.
- 80×24 terminalda buzilmasin; tor joyda ustunlar qisqaradi, kesiladi (`…`).
- Foydalanuvchiga ko'rinadigan matnlar o'zbek tilida va izchil; hajm, vaqt,
  tezlik — umumiy formatlovchilar orqali.

## 5. Tashqi buyruqlar va manbalar

- `exec.Command` faqat qat'iy argumentlar bilan, **shell'siz** (`sh -c` yo'q),
  har doim `context` timeout bilan.
- Tashqi ma'lumot (claude tomonidan kelgan JSON, `.caddy` fayllar, modem XML,
  `/proc`) ishonchsiz: hajmi chegaralanadi, parse xatosi ishlanadi, qiymatlar
  tekshiriladi.
- **Maxfiylik:** `.env`, kalitlar, compose `environment` qiymatlari o'qilmaydi va
  ko'rsatilmaydi (faqat nomlari kerak bo'lsa). Modem cookie/token logga tushmaydi.
- **Modem:** bir vaqtda bitta USSD (abstract socket lock, diskka yozmasdan), bitta
  jarayonda 5 daqiqada ko'pi bilan bir marta (D-006).
- **Imtiyozlar:** TUI shranet sifatida ishlaydi; claude tomoni faqat
  `collect` rejimi orqali, faqat o'qiydi, hech narsani o'zgartirmaydi.
  O'rnatilgan binary `root:root 0755` (claude yoza olmasin) — D-003.
- pcinfo hech qachon konteyner, tarmoq, fayl yoki proyektni o'zgartirmaydi:
  faqat ko'rsatadi.

## 6. Xatolar

- Xato qaytariladi yoki ishlanadi, yutib yuborilmaydi; `fmt.Errorf("…: %w", err)`
  bilan kontekst qo'shiladi, tekshirish `errors.Is/As` bilan.
- UI'da xato qisqa va tushunarli matn bilan; ichki tafsilot (stack, to'liq yo'l)
  ko'rsatilmaydi. Log fayl yo'q: dastur diskka hech narsa yozmaydi (D-007).

## 7. Nomlash va izohlar

- Nomlar domen tilida va aniq; mantiqiy qiymat savol kabi o'qiladi (`isActive`).
- Izoh "nega" ni tushuntiradi. Izohlar o'zbek tilida, kod identifikatorlari inglizcha.

## 8. Testlar

- Har kollektor va parser — jadvalga asoslangan test, `testdata/` dagi haqiqiy
  namunalar bilan (`.caddy`, `/proc/net/route`, modem XML, `docker` javoblari).
  Kollektor manbani (fayl tizimi ildizi, HTTP manzil, buyruq natijasi) parametr
  sifatida oladi, shunda test haqiqiy tizimga tegmaydi.
- Avval xato holatlar va chegaralar, keyin "baxtli yo'l".
- TUI: model `Update` mantiqi xabarlar bilan testlanadi (ekran snapshot'i emas).
- Bug tuzatilsa — avval uni takrorlaydigan test.

## 9. Build va tekshiruv (hammasi Docker'da)

```bash
# testlar + vet + format tekshiruvi
docker run --rm -v "$PWD":/src -w /src -v /go/pkg/mod -v /root/.cache golang:1.26.10 \
  sh -c 'test -z "$(gofmt -l .)" && go vet ./... && go test ./...'

# statik binary
docker run --rm -v "$PWD":/src -w /src -v /go/pkg/mod -v /root/.cache -e CGO_ENABLED=0 \
  -u "$(id -u):$(id -g)" -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomod \
  golang:1.26.10 go build -trimpath -ldflags='-s -w' -o bin/pcinfo ./cmd/pcinfo
```

"Tayyor" deyishdan oldin testlar, vet, gofmt o'tadi va natija ko'rsatiladi.
`bin/` gitignore'da.

## 10. Jarayon

- Katta ish: spec → reja → kichik qadamlar → review. Talab noaniq bo'lsa — so'ra.
- Har mantiqiy qadam tekshirilgach commit va push (global qoida). Commit kichik,
  xabarida "nega".
- Muhim qaror `docs/decisions.md` ga yoziladi: raqam, sana, kontekst, qaror,
  sabab, ochiq xavflar, holat. Agent qarori — **TAKLIF**, foydalanuvchi
  tasdiqlagach — **TASDIQLANGAN**.
- Ish oxirida review: to'g'rilik va xavfsizlik / soddalik va o'qiluvchanlik.
- Hisobotda haqiqat: nima qilindi, nima qilinmadi, nima o'tmadi.

## 11. Topshirishdan oldin

- Kod uslubga mos, so'ralgan doiradan chiqmagan; sehrli qiymat, takror, o'lik kod yo'q.
- `Update`/`View` da bloklovchi IO yo'q; ranglar faqat `theme` dan; har panelda to'rt holat.
- Tashqi buyruqlar shell'siz va timeout bilan; maxfiy qiymat ko'rsatilmaydi.
- gofmt, vet, testlar Docker'da o'tgan; spec va qarorlar yangilangan.

# pcinfo

Shu PC haqida htop'ga o'xshash terminal dasturi (Go, Bubble Tea): tizim, jarayonlar,
Docker, proyektlar va preview'lar, internet liniyalari (optika / USB modem), modem balansi.
**Faqat o'qiydi** — yagona istisno: ishga tushganda modemga balans USSD'si (`*107#`).

- Spetsifikatsiya: [docs/spec.md](docs/spec.md)
- Qarorlar: [docs/decisions.md](docs/decisions.md)
- Kod qoidalari: [CLAUDE.md](CLAUDE.md)

## Ishlatish

```
pcinfo               # TUI
pcinfo --no-ussd     # balans USSD'sini yubormasdan (keyin Tarmoq tabida u)
pcinfo --public-ip   # har liniyaning tashqi IP'sini so'rash (api.ipify.org ga so'rov)
pcinfo --version
```

Tugmalar: `1-5` / `Tab` — bo'limlar, `↑↓ PgUp PgDn Home End` — ro'yxat, sichqoncha —
tab, qator, ustun sarlavhasi (saralash), g'ildirak. Jarayonlar: `/` qidiruv, `c m p n` saralash,
`r` teskari. Proyektlar: `Enter`/klik — ochish, `a` — hammasini ochish/yopish.
Tarmoq: `u` — balansni qayta so'rash (5 daqiqada bir marta). `q` — chiqish.

## O'rnatish (shranet, bir marta)

Binary'ni **o'zingizning klonigizdan** build qiling: sudo orqali ishlaydigan fayl claude
yoza oladigan joydan kelmasligi kerak (D-003).

```bash
git clone git@github.com:shranet/pcinfo.git && cd pcinfo
docker run --rm -v "$PWD":/src -w /src -e CGO_ENABLED=0 golang:1.26.10 \
  go build -trimpath -ldflags="-s -w -X main.version=$(git describe --tags --always)" -o bin/pcinfo ./cmd/pcinfo

sudo install -o root -g root -m 0755 bin/pcinfo /usr/local/bin/pcinfo
echo 'shranet ALL=(claude) NOPASSWD: /usr/local/bin/pcinfo collect --json' | sudo tee /etc/sudoers.d/pcinfo
sudo chmod 0440 /etc/sudoers.d/pcinfo
sudo visudo -cf /etc/sudoers.d/pcinfo
```

Tekshirish: `sudo -n -u claude -- /usr/local/bin/pcinfo collect --json | head -c 200`.
Yangilashda faqat `install` qadamini takrorlang. sudoers qoidasi faqat aynan
`collect --json` ga ruxsat beradi; TUI sudo'dan oldin faylning egasi root ekanini va
boshqalar yoza olmasligini tekshiradi, aks holda claude ma'lumotini so'ramaydi.

## Ishlab chiqish (hammasi Docker'da)

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.26.10 \
  sh -c 'test -z "$(gofmt -l .)" && go vet ./... && go test ./...'
```

Haqiqiy tizim bilan har tabni matn qilib ko'rish (USSD yuborilmaydi):

```bash
docker run --rm -v "$PWD":/src -w /src -e CGO_ENABLED=0 golang:1.26.10 go test -c -o bin/tui.test ./internal/tui
PCINFO_LIVE=1 bin/tui.test -test.run Live
```

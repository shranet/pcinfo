# Qarorlar jurnali

Holatlar: **TAKLIF** (agent taklif qilgan) → **TASDIQLANGAN** (foydalanuvchi rozi).

## D-001 · 2026-10-29 · Til va TUI stek
- **Kontekst:** terminalda sichqoncha va ranglar kerak; hostga runtime o'rnatib bo'lmaydi.
- **Qaror:** Go, statik binary (Docker'da build). Bubble Tea v2 + lipgloss v2 + bubbles v2,
  bubblezone (sichqoncha zonalari), ntcharts (grafiklar), gopsutil v4, Docker SDK.
- **Sabab:** bitta binary hostga runtime'siz tushadi; Docker SDK va gopsutil Go'da eng yetuk.
  Alternativa Rust + ratatui — yozish sekinroq.
- **Holat:** TASDIQLANGAN (foydalanuvchi: "stack ok").

## D-002 · 2026-10-29 · Ikki rejimli bitta binary
- **Kontekst:** TUI shranet sifatida ishlaydi, lekin claude user'ning rootless Docker'i,
  proyektlari va router fayllari faqat claude'ga ko'rinadi.
- **Qaror:** `pcinfo` (TUI) va `pcinfo collect --json` (faqat o'qiydigan kollektor).
  TUI sekin davrda `sudo -n -u claude /usr/local/bin/pcinfo collect --json` ni chaqiradi.
  JSON kontrakti `internal/snapshot` da, `schema` versiyasi bilan.
- **Sabab:** TUI'ning o'zi imtiyozsiz; claude huquqi faqat tor, o'qiydigan rejimda.
- **Holat:** TASDIQLANGAN.

## D-003 · 2026-10-29 · sudo ruxsati va binary egasi
- **Qaror:** `/etc/sudoers.d/pcinfo`:
  `shranet ALL=(claude) NOPASSWD: /usr/local/bin/pcinfo collect *`.
  Binary `/usr/local/bin/pcinfo`, `root:root 0755`.
- **Sabab:** claude binary'ga yoza olsa, uni almashtirib shranet huquqini olishi mumkin
  (shranet uni o'z nomidan ishga tushiradi).
- **Ochiq xavf:** o'rnatishni (root kerak) shranet o'zi qiladi; pcinfo uni tekshirib,
  binary claude yoza oladigan joyda bo'lsa ogohlantiradi.
- **Holat:** TAKLIF.

## D-004 · 2026-10-29 · Proyektlarni guruhlash
- **Kontekst:** `projects/` papkalari workspace (ichida bir nechta repo); compose nomi papka
  nomidan farq qiladi (`adbus-share` → `adbus`); ba'zi konteyner label'larida papka qayta
  nomlanishidan oldingi yo'l qolgan (`projects/winshop` → `ruvex-winshop`); `.caddy`
  fayl nomi host bilan mos kelmasligi mumkin (`inbox.caddy` → `files.long.uz`).
- **Qaror:** workspace = `projects/` ning birinchi darajali papkasi. Konteyner →
  workspace: `com.docker.compose.project.working_dir` label'i; yo'l mavjud bo'lmasa —
  compose nomi bo'yicha `<nom>`, `ruvex-<nom>`, `*-<nom>` papka. Preview → compose:
  `.caddy` ichidagi `reverse_proxy <konteyner>:<port>` dan konteyner nomi.
  `# pvds-public` qatorlari va `CLAUDE.md` dagi `preview: local` ham o'qiladi.
- **Holat:** TAKLIF.

## D-005 · 2026-10-29 · Internet liniyasini aniqlash
- **Kontekst:** ikkita default route: `enp6s0` via `192.168.100.1` (optika, metric 100) va
  USB modem via `192.168.8.1` (metric 700). `tailscale0` `100.64.x` manzilli — uni optika
  deb adashtirmaslik kerak.
- **Qaror:** liniya turi gateway/subnet bo'yicha (`192.168.8.0/24` → USB modem,
  `192.168.100.0/24` → optika) konstantalarda; aktiv liniya — eng kichik metric'li,
  ishlayotgan default route (`/proc/net/route`, netlink kutubxonasisiz). Har liniya
  manba IP'sidan `1.1.1.1:443` ga TCP ulanish bilan tekshiriladi (sudo'siz).
  Public IP — ishga tushishda bir marta, flag bilan o'chiriladi.
- **Holat:** TAKLIF.

## D-006 · 2026-10-29 · Modem USSD
- **Qaror:** ishga tushishda fonda bitta `*107#`; holatlar ekranda jonli. Natija keshi
  `/run/user/<uid>/pcinfo/ussd.json` (tmpfs), 5 daqiqa ichida qayta yuborilmaydi;
  `flock` bilan bir vaqtda bitta USSD. Signal/holat/trafik har bir necha soniyada
  (USSD emas, cheklov yo'q).
- **Holat:** TAKLIF.

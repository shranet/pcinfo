# Qarorlar jurnali

Holatlar: **TAKLIF** (agent taklif qilgan) → **TASDIQLANGAN** (foydalanuvchi rozi).

## D-001 · 2026-10-29 · Til va TUI stek
- **Kontekst:** terminalda sichqoncha va ranglar kerak; hostga runtime o'rnatib bo'lmaydi.
- **Qaror:** Go, statik binary (Docker'da build). Bubble Tea v2 + lipgloss v2, gopsutil v4.
- **Sabab:** bitta binary hostga runtime'siz tushadi. Alternativa Rust + ratatui — yozish sekinroq.
- **Holat:** TASDIQLANGAN (foydalanuvchi: "stack ok").

### D-001a · 2026-10-29 · Kutubxonalar kamaytirildi
- **Qaror:** dastlabki ro'yxatdagi `bubbles`, `bubblezone`, `ntcharts` va Docker SDK ishlatilmaydi.
  Docker — standart `net/http` bilan unix socket orqali, faqat GET (`internal/collect/dockerapi`).
  Sichqoncha zonalari, jadval, sparkline — o'zimizning ~100 qatorlik kod (`internal/tui/widgets.go`).
- **Sabab:** xavfsizlik birinchi o'rinda: kamroq bog'liqlik — kamroq ta'minot zanjiri xavfi.
  Docker SDK katta va yozuvchi metodlarga ega; o'z GET-mijozimiz konteynerni o'zgartira olmasligi
  kod darajasida kafolatlangan (test ham tekshiradi).
- **Holat:** TAKLIF.

## D-002 · 2026-10-29 · Ikki rejimli bitta binary
- **Kontekst:** TUI shranet sifatida ishlaydi, lekin claude user'ning rootless Docker'i,
  proyektlari va router fayllari faqat claude'ga ko'rinadi.
- **Qaror:** `pcinfo` (TUI) va `pcinfo collect --json` (faqat o'qiydigan kollektor).
  TUI `sudo -n -u claude -- /usr/local/bin/pcinfo collect --json` ni chaqiradi.
  JSON kontrakti `internal/snapshot` da, `schema` versiyasi bilan. TUI claude nomidan
  ishlasa, sudo'siz shu jarayonning o'zida yig'adi.
- **Sabab:** TUI'ning o'zi imtiyozsiz; claude huquqi faqat tor, o'qiydigan rejimda.
  claude bilan bog'liq hamma narsa (git, repo fayllari, caddy) faqat claude jarayonida
  o'qiladi — shranet jarayoni claude nazoratidagi git konfiguratsiyasini hech qachon ishga tushirmaydi.
- **Holat:** TASDIQLANGAN.

## D-003 · 2026-10-29 · sudo ruxsati va binary egasi
- **Qaror:** `/etc/sudoers.d/pcinfo` — argumentlar aniq, wildcard'siz:
  `shranet ALL=(claude) NOPASSWD: /usr/local/bin/pcinfo collect --json`.
  Binary `/usr/local/bin/pcinfo`, `root:root 0755`. `collect` rejimi `--json` dan boshqa
  argumentni qabul qilmaydi.
- **Sabab:** claude binary'ga yoza olsa, uni almashtirib shranet huquqini olishi mumkin
  (shranet uni o'z nomidan ishga tushiradi). TUI sudo'dan oldin faylni tekshiradi (egasi root,
  symlink emas, guruh/boshqalar yoza olmaydi; papkasi ham) va shart bajarilmasa sudo'ni
  chaqirmaydi (fail-closed). Ishlayotgan binary boshqa foydalanuvchiga tegishli yoki
  hamma yoza oladigan bo'lsa — Tizim tabida ogohlantirish.
- **Ochiq xavf:** o'rnatishni (root kerak) shranet o'zi qiladi; binary'ni shranet o'z
  klonidan build qilgani ma'qul (README).
- **Holat:** TASDIQLANGAN ("go hammasini").

## D-004 · 2026-10-29 · Proyektlarni guruhlash
- **Kontekst:** `projects/` papkalari workspace (ichida bir nechta repo); compose nomi papka
  nomidan farq qiladi (`adbus-share` → `adbus`); ba'zi konteyner label'larida papka qayta
  nomlanishidan oldingi yo'l qolgan (`projects/winshop` → `ruvex-winshop`); `.caddy`
  fayl nomi host bilan mos kelmasligi mumkin (`inbox.caddy` → `files.long.uz`).
- **Qaror:** workspace = `projects/` ning birinchi darajali papkasi. Konteyner →
  workspace: `com.docker.compose.project.working_dir`; yo'l eskirgan bo'lsa — papka yoki
  compose nomi bo'yicha `<nom>`, `ruvex-<nom>`, `*-<nom>` (faqat bitta mos kelsa). Preview →
  compose: `.caddy` ichidagi `reverse_proxy <konteyner>:<port>`. `# pvds-public` qatorlari va
  `CLAUDE.md` dagi `preview: local` ham o'qiladi. Ogohlantirishlar: router tarmog'idagi
  DB/Redis/MinIO, hostga ochiq port, `mem_limit` yo'q, o'chiq konteynerga yo'nalish,
  `preview: local` proyekt long.uz'da.
- **Holat:** TASDIQLANGAN.

## D-005 · 2026-10-29 · Internet liniyasini aniqlash
- **Kontekst:** ikkita default route: `enp6s0` via `192.168.100.1` (optika, metric 100) va
  USB modem via `192.168.8.1` (metric 700). `tailscale0` `100.64.x` manzilli — uni optika
  deb adashtirmaslik kerak.
- **Qaror:** liniya turi gateway/subnet bo'yicha (`192.168.8.0/24` → USB modem,
  `192.168.100.0/24` → optika); aktiv liniya — ishlayotgan default route'lar ichida eng kichik
  metric'lisi (`/proc/net/route`). Har liniya `SO_BINDTODEVICE` bilan `1.1.1.1:443` ga TCP
  ulanish ochib-yopib tekshiriladi (sudo'siz, ma'lumot yuborilmaydi). Public IP —
  **standart holatda o'chiq**, faqat `--public-ip` bilan (tashqi servisga so'rov ketadi).
- **Holat:** TASDIQLANGAN.

## D-006 · 2026-10-29 · Modem USSD
- **Qaror:** ishga tushishda fonda bitta `*107#` (`--no-ussd` bilan o'chadi); holatlar ekranda
  jonli. Bir vaqtda bitta USSD — foydalanuvchining shaxsiy `/run/user/<uid>` papkasiga
  `flock` (fayl yaratilmaydi; boshqa foydalanuvchi uni band qila olmaydi — review'da
  umumiy abstract socket'ni claude egallab olishi mumkinligi topilgan). Bitta jarayon ichida qayta so'rash (`u`) 5 daqiqada bir marta.
- **O'zgarish:** dastlabki taklifdagi `/run/user/<uid>/pcinfo/ussd.json` keshi olib tashlandi —
  "faqat readonly, ortiqcha yozmasin" talabi.
- **Ochiq xavf:** pcinfo'ni 5 daqiqa ichida qayta-qayta ochish har safar USSD yuboradi
  (kesh yo'q). Tez-tez ochilsa — `--no-ussd` bilan ishga tushirib, kerak bo'lganda `u`.
- **Holat:** TASDIQLANGAN (kesh o'zgarishi — TAKLIF).

## D-007 · 2026-10-29 · Faqat o'qish kafolatlari
- **Qaror:**
  - Docker: mijozda faqat `GET` (`dockerapi.get`), konteyner ID formati tekshiriladi.
  - git: `/usr/bin/git --no-optional-locks -c core.fsmonitor=false status --ignore-submodules=all`,
    `GIT_OPTIONAL_LOCKS=0`, `GIT_NO_LAZY_FETCH=1`, `GIT_ATTR_SOURCE=<bo'sh daraxt>` — indeks
    yozilmaydi (test tekshiradi), filtr/submodule/lazy fetch ishga tushmaydi.
  - Fayllar: faqat o'qiladi, hajmi chegaralangan, symlink'larga kirilmaydi.
  - Hech qanday kesh, log yoki vaqtinchalik fayl yozilmaydi.
  - Yagona tashqi o'zgarish — modemga USSD yuborish (talab bo'yicha).
  - Barcha tashqi matnlar (claude JSON, /proc, Docker, modem, xato matnlari) terminalga
    chiqishdan oldin `safetext` bilan tozalanadi (ESC/CSI/OSC, C1, bidi).
- **Holat:** TAKLIF.

## D-008 · 2026-10-29 · claude tomonini so'rash chastotasi
- **Kontekst:** har `sudo` chaqiruvi auth.log va journal'ga yozuv qoldiradi (sudo-rs).
- **Qaror:** Proyektlar yoki Docker tabi ochiq bo'lsa — har 10 s, aks holda daqiqada bir marta.
- **Holat:** TAKLIF.

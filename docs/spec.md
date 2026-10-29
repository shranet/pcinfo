# pcinfo — spetsifikatsiya

## Maqsad
Shu PC (shranet ishlatadi, claude user'i proyektlarni yuritadi) holatini bitta
terminal oynasida ko'rsatish: resurslar, Docker, proyektlar va ularning preview'lari,
internet liniyalari va modem balansi. Faqat ko'rsatadi, hech narsani o'zgartirmaydi.

## Ishga tushirish
- `pcinfo` — TUI (shranet sifatida).
- `pcinfo collect --json` — claude tomoni (sudo orqali, D-002/D-003).

## Tablar
| Tab | Mazmuni |
|---|---|
| Tizim | CPU (yadrolar, sparkline), RAM/swap, disklar, temperatura, uptime, load |
| Jarayonlar | htop'dagi kabi jadval: saralash, qidiruv, foydalanuvchi bo'yicha filtr |
| Proyektlar | workspace → compose → konteynerlar → preview hostlari; git holati; ogohlantirishlar |
| Docker | shranet va claude docker'lari alohida: konteynerlar, image, volume, tarmoq, resurs |
| Tarmoq | uplink'lar (optika / USB / VPN), aktiv liniya, tezlik; modem signali va USSD |

Yuqorida doimiy status satri: CPU, RAM, ishlayotgan konteynerlar, aktiv liniya, modem MB.

## Proyektlar tabi (D-004)
Har workspace qatori: holat (`n/m up`), RAM/CPU yig'indisi, preview hostlari (public yo'llar soni).
Ochilganda: compose proyektlar va konteynerlar; repolar (`↑N` push qilinmagan, `dirty`);
`deploy.json` bo'lsa — prod teg.
Ogohlantirishlar: `long-preview` ga ulangan DB/Redis; hostga ochilgan `ports:`;
`mem_limit` yo'q; `.caddy` o'chiq konteynerga ishora qiladi.

## Tarmoq tabi (D-005, D-006)
- Liniyalar: tur, interfeys, IP, gateway, metric, aktivmi, TCP tekshiruv (ms), tezlik, public IP.
- USB orqali chiqilsa — status satrida ogohlantirish (limitli trafik).
- Modem: signal (RSSI/RSRP/SINR), tarmoq turi, ulanish holati, trafik;
  USSD `*107#`: `sessiya → yuborildi → kutilmoqda (Ns/40s) → tayyor | xato`,
  parse: balans, MIN, MB, SMS + xom matn. `u` — qo'lda yangilash (5 daqiqalik cheklov bilan).

## Bosqichlar
1. Skelet: `cmd/pcinfo`, rejimlar, `theme`, tablar karkasi, status satri.
2. Tarmoq: liniyalar + modem (signal, USSD, kesh, lock).
3. Proyektlar: `collect` (claude tomoni), guruhlash, preview, git.
4. Tizim va Jarayonlar.
5. Docker tabi.
6. O'rnatish yo'riqnomasi (sudoers, binary egasi), review.

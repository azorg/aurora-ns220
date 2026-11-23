# 17. Debian-chroot на NS220 как рабочий стенд: pi, ClearSSH, cnet

Дата: 2026.11.23. Цель: превратить Debian-chroot на планшете
(`/home/chroot/debian`, см. doc/11) в постоянный стенд — с Node.js+pi,
SSH-сервером ClearSSH и узлом оверлея cnet (`node-ns220`), доступом к
домашнему каталогу Aurora и автозапуском при загрузке.

Проверено на живом устройстве (NS220, Aurora 4.0.2.340, ARMv7).

## 17.1. Что где лежит

Хост (Aurora):

| Путь | Назначение |
|---|---|
| `/usr/local/bin/ns220-svc` | управление стендом: `start|stop|restart|status` (mount-ы chroot + запуск сервисов внутри) |
| `/etc/systemd/system/ns220-chroot.service` | автозапуск стенда при загрузке (oneshot, RemainAfterExit) |
| `/usr/local/bin/debian-chroot` | вход в chroot; сам монтирует `/home/user` → `/home/aurora`, `/run/media/user` → `/media/aurora`, уважает флаг `/run/ns220-svc.active` |
| `/etc/sudoers.d/20-user-nopasswd` | `user ALL=(ALL) NOPASSWD: ALL` — `sudo` без пароля |
| `/etc/hosts` | `fd00:37a:...:978e node-ns220` (оверлейный адрес) |

В chroot (`/home/chroot/debian`):

| Путь | Назначение |
|---|---|
| `/opt/ns220/{start,stop,status}.sh` | запуск/остановка/статус сервисов |
| `/opt/node` → `/opt/node-v22.23.4-linux-armv7l` | Node.js 22.23.4 (linux-armv7l) |
| `/usr/local/bin/{node,npm,npx,pi}` | симлинки Node/npm/npx и pi |
| `/root/.pi/agent/` | конфигурация pi, скопированная с ПК (см. 17.4) |
| `/usr/local/bin/{cssh,csshd,cdialog}` | ClearSSH 0.0.22 (root-установка) |
| `/etc/cssh/csshd.yaml` | конфигурация csshd: порт **222**, privsep monitor |
| `/etc/pam.d/csshd` | PAM-сервис для парольной аутентификации |
| `/usr/local/bin/cnet` (+ `cnet-node`, `cnet-leaf`, `cnet-point`, `cnet-service`) | cnet 0.1.28 armv7 |
| `/etc/cnet/config/{node.yaml,node.key,node.cert,node.jwt}` | профиль узла, ключ, цепочка, паспорт |
| `/etc/cnet/certs/` | root.crt, ca.crt, ca-chain.pem, `peers/` (сертификаты соседей) |
| `/var/lib/cnet/store/` | хранилище: pubkey соседей и издателя паспортов |
| `/var/log/{clearssh,cnet}/` | журналы сервисов |
| `/home/aurora` | bind-монтирование `/home/user` Aurora (документы, загрузки, скриншоты) |
| `/media/aurora` | bind-монтирование `/run/media/user` (SD-карта и медиа) |
| `/aurora`, `/root/aurora` | симлинки на `/home/aurora` |

## 17.2. Управление стендом (systemd: три юнита)

Стенд разделён на три юнита — по одному на слой:

| Юнит | Что делает |
|---|---|
| `ns220-chroot.service` | **только монтирования**: remount exec `/home`, bind `/proc /sys /dev /dev/pts`, tmpfs `/tmp`, bind `/home/user` → `/home/aurora`, `/run/media/user` → `/media/aurora`; ставит флаг `/run/ns220-svc.active` (`ExecStart=ns220-svc mount`) |
| `csshd-chroot.service` | ClearSSH-сервер в chroot: `debian-chroot csshd -D -f /etc/cssh/csshd.yaml` (порт **2222**, см. 17.3) |
| `cnet-chroot.service` | узел cnet в chroot: `debian-chroot cnet run --role node --config /etc/cnet/config/node.yaml --iptun=on` (17.5) |

```sh
systemctl status  ns220-chroot csshd-chroot cnet-chroot
sudo systemctl restart cnet-chroot        # перезапуск узла cnet
sudo ns220-svc status                     # сводка: порт, peers, /home/aurora
sudo ns220-svc start                      # «всё в одном» без systemd-юнитов
sudo ns220-svc stop                       # остановить оба сервиса и размонтировать
```

`debian-chroot` (интерактивный вход) поднимает нужные монтирования сам и
**не размонтирует** то, что держит стенд: пока существует
`/run/ns220-svc.active`, монтирования остаются. Поэтому csshd-chroot и
cnet-chroot могут работать одновременно, а вход в chroot их не ломает.

Пример юнита для cnet лежит в репозитории cnet:
`deploy/cnet-chroot.service` (копия — `scripts/cnet-chroot.service`).

## 17.3. ClearSSH (csshd, порт 222)

* Установка: официальный инсталлятор ClearSSH в режиме `root`,
  `PORT=2222`, `COMPONENTS=both`, `PROVIDER=gocrypto`,
  `CGO_ENABLED=0`, `GOARCH=arm`.
* В chroot нет systemd, поэтому юнит `csshd.service` не активируется —
  сервер запускает `/opt/ns220/start.sh` (nohup, pid в `/run/csshd.pid`,
  лог `/var/log/clearssh/csshd.out`).
* Конфигурация `/etc/cssh/csshd.yaml`: `network.ports: [2222]`,
  `security.privilege-separation: monitor`, `security.service-user:
  clearssh`, `authentication.password-backend: pam`,
  `permit-root-login: yes` (dev-стенд; root в chroot — пароль `1234`,
  как у учётной записи Aurora).
* Доступ (`user` — см. 17.9; пароль `1234`):
  `cssh -p 2222 user@192.168.2.15` (USB RNDIS),
  `cssh -p 2222 user@192.168.1.241` (Wi-Fi), `root` — тем же портом.
  Проверено и OpenSSH-клиентом: `ssh -p 2222 user@...`.
* Сборка armv7: `scripts/build-armv7-chroot.sh clearssh` — кросс-сборка
  требует патча go-fuse (`patches/go-fuse-2.12.1-splice-32bit.patch`,
  подключается через `-modfile`, без правки `go.mod`) и правки
  ELF-interpreter (`patchelf --set-interpreter /lib/ld-linux-armhf.so.3`).

## 17.4. Node.js 22 и pi

* Node.js **22.23.4** linux-armv7l (собран для glibc ≥ 2.28, Debian 12
  armhf подходит): `/opt/node-v22.23.4-linux-armv7l`, симлинк
  `/opt/node`; в PATH — `/usr/local/bin/{node,npm,npx}`.
  Требуется пакет `libatomic1` (иначе `node: error while loading shared
  libraries: libatomic.so.1`).
* pi **1.6.0** установлен глобально: `npm install -g
  @earendil-works/pi-coding-agent@latest`; бинарь
  `/usr/local/bin/pi` → `/opt/node/bin/pi`. Проверено: `pi --version`
  → 1.6.0 на armv7.
* Настройки скопированы с ПК (`~/.pi/agent` → `/root/.pi/agent`):
  `settings.json`, `models.json`, `models-store.json`, `auth.json`,
  `trust.json`, `extensions/` (3 шт.), `skills/`, `sounds/`.
  Каталоги `install/` (Node/managed-релиз) и `sessions/` (история
  сессий) НЕ копировались — они привязаны к платформе/объёму.
* Поиск: pi сам скачивает `rg`/`fd` только для x64/arm64; на armv7
  используются системные пакеты Debian `ripgrep` (13.0.0) и
  `fd-find` (8.6.0, бинарь `fdfind` — pi его находит).
* Внутри pi доступен `/home/aurora` — можно читать/писать файлы
  пользователя Aurora (документы, загрузки, скриншоты).

## 17.5. cnet: узел node-ns220

* Бинарь: cnet **0.1.28+8863876** (armv7, сборка с правкой BUG 0052 —
  переполнение `BufSizeDecode` на 32-битных платформах).
* Идентичность: ID `fd00:037a:b722:dbd2:f625:afc2:509e:978e`
  (HEX `fd00037ab722dbd2f625afc2509e978e`), роль node, сеть `cnet-0.1`,
  кластер `default`, EdDSA, mining `fd00::/20`.
* Сертификат: CNET Intermediate CA site-a (SAN node-ns220/localhost/
  127.0.0.1, EKU serverAuth+clientAuth).
* Паспорт (JWT): выпущен epc-регистратором (issuer kid
  `B76DB8C1...`), `jwt-file: /etc/cnet/config/node.jwt`, срок до
  2027-11-23.
* Транспорт: `listen hijacks://0.0.0.0:8443/cnet`; `dial` — два входа:
  `hijacks://95.181.161.53:8443/cnet` (node-de, основной) и
  `hijacks://freelab.su:8443/cnet` (node-freelab, резервный).
* Прокси: `overlay-proxy` на **`tcp://127.0.0.1:1080`** (HTTP и SOCKS5,
  auth `user:12345678`), `next-proxy-url =
  overlay://fd00182c7f8ead8412771f6e2f79edad:1080` — **выход по
  умолчанию через node-de**. Проверено: `curl -x socks5h://user:12345678@
  127.0.0.1:1080 https://api.ipify.org` и `-x http://...` → 95.181.161.53;
  без пароля — отказ. Прокси умеет CONNECT и на оверлейные IPv6-адреса,
  поэтому через него доступен и HyperQwen на node-bigpc (проверено:
  `/v1/models` → 200). Свой `outbound-proxy` (overlay→tcp) выключен:
  узел — потребитель выхода (D-002).
* iptun: `cnet0`, mask 16 (`fd00::/16`), mtu 1500, port 1917;
  `/dev/net/tun` в chroot доступен (bind из `/dev`).
* Консоль: `tcp://127.0.0.1:2323` (пароль кластера), история
  `/root/history/node`.
* Store: `/var/lib/cnet/store`, `on-memory-only: false` (иначе
  pubkey соседей с диска не читаются — см. 17.6).

## 17.6. Грабли включения нового узла (по шагам, что чинили)

1. **Store.** При `on-memory-only: true` каталог store игнорируется,
   и проверка паспорта соседа падает с `key or certificate not found`.
   Нужно `on-memory-only: false` и наполнение каталога:
   `cnet certcache --add <pub.pem> --store-dir /var/lib/cnet/store
   --store-on-memory-only off`.
2. **Ключ издателя паспортов.** В store должен лежать pubkey
   регистратора — файл `b76db8c1.0` (kid `B76DB8C1...`). Без него
   свой узел не проверяет JWT соседей.
3. **Паспорт.** Узлы работают с `auth.policy: x509+jwt`, поэтому
   новому узлу нужен JWT. Порядок: на epc
   `cnet pki invite new --role node --name <имя> --mining-cidr
   fd00::/20 --network cnet-0.1 --issuer-key ~/.cnet/pki/keys/issuer.key
   --out invite.txt`; на узле `cnet pki request new --invite invite.txt
   --key-file node.key --out request.txt`; на epc
   `cnet pki request approve -yes -in request.txt --out passport.txt
   --issuer-key ... --days 366`. В `node.yaml` — `jwt-file:` с СЫРЫМ
   JWT (одна строка `eyJ...`), не armored CNET PASS.
4. **Цепочка сертификата.** `node.cert` = лист + промежуточный ЦС
   (без корня). Паспортный `chain.pem` содержит лист+ЦС+корень —
   корень надо убрать.
5. **Взаимный store.** Если узел сам ДИАЛИТ соседа, ему нужен pubkey
   соседа; соседу — pubkey нового узла (иначе
   `peer X5c: x509: certificate signed by unknown authority`).
   Наполнение на обеих сторонах — `cnet certcache --add`.

## 17.6.1. iptun: что проверено (и что осталось)

* `cnet0` поднимается (`fd00:037a:.../16`, mtu 1500), маршрут `fd00::/16
  dev cnet0`, `/dev/net/tun` в chroot доступен.
* **Туннель работает в обе стороны**: `ping6` (через обёртку
  `/usr/local/bin/ping`, от uid 100000) от ns220 до epc — 2/2, до
  node-bigpc — 3/3 (0% потерь, RTT 290–360 мс); с epc/bigpc до нас
  cnet-ping и ICMP тоже проходят. TCP epc → ns220:2222 соединяется.
* **Не работает TCP именно с node-bigpc** (и на всех его портах:
  22/80/2222/18020 — включая HyperQwen): SYN до bigpc доходит (его
  cnet0 rx растёт), но ответа нет, SYN-RECV на bigpc не появляется;
  при этом TCP ns220 → epc даёт SYN+RST мгновенно. Заведён
  **BUG 0055** в `~/work/cnet/doc/ai/bugs/0055.iptun-node-ns220-oneway-armv7.txt`
  (там же гипотезы и что нужно снять на bigpc).
* Наблюдение по режиму tun: у кластера `batchSize=128` (IFF_VNET_HDR,
  GSO/checksum offload), у ns220 на ядре MTK 3.18 — `batchSize=1`
  (offload не включается). К ICMP это не относится (он не офлоадится),
  но для TCP может быть значимо — проверяется в BUG 0055.


## 17.6.2. ВАЖНО: на NS220 не работает ИСХОДЯЩИЙ IPv6-TCP (ядро MTK 3.18)

* Проверено минимальным Go-тестом на самом планшете:
  `net.Dial("tcp6", "[::1]:3342")` → **i/o timeout**,
  `net.Dial("tcp4", "127.0.0.1:3343")` → OK. Счётчики:
  IPv6-loopback — `ActiveOpens +1, PassiveOpens +0, AttemptFails +2,
  OutRsts +2`; IPv4-loopback — `PassiveOpens +1`.
  ICMPv6 (в т.ч. через оверлей) и **входящий** IPv6-TCP работают —
  сломан именно клиентский путь TCPv6 в ядре Aurora 4.0.2.340
  (MTK 3.18.79). Не chroot, не ip6tables, не rpfilter, не чек-суммы.
* Следствие: приложение на планшете **не может открыть** IPv6-TCP
  соединение к сервису в оверлее (HyperQwen на node-bigpc и т.п.).
* Рабочий обход: **cnet-прокси** `127.0.0.1:1080` (HTTP+SOCKS5,
  user:12345678) — приложение ходит по IPv4-loopback, а оверлейный
  трафик cnet несёт внутри своих TCP-каналов. Через прокси работают
  и внешние API, и HyperQwen (pi настроен именно так, см. 17.10).
* Диагностика и доказательства — `~/work/cnet/doc/ai/bugs/0055`
  (раздел 12), там же что исключено (chroot, ip6tables, rpfilter,
  netns, маршруты, контрольные суммы).

## 17.7. Сборка бинарников для armv7

```sh
# на хосте (x86_64), из каталога aurora-ns220:
scripts/build-armv7-chroot.sh all          # cnet + clear-ssh -> /tmp/armbuild
scripts/build-armv7-chroot.sh cnet         # только cnet
scripts/build-armv7-chroot.sh clearssh     # только cssh/csshd/cdialog
```

Особенности (см. 17.3): go-fuse патч через `-modfile`; `patchelf` для
интерпретатора clear-ssh. Бинарь cnet — статический, копируется в
chroot как есть.

## 17.9. Сеть и DNS в chroot (важно)

1. **DNS.** В Aurora `/etc/resolv.conf` — заглушка Connection Manager
   (`nameserver ::1`, `127.0.0.1`). Из chroot она ненадёжна, поэтому
   `ns220-svc`/`debian-chroot` пишут в chroot свой `resolv.conf`:
   `8.8.8.8`, `1.1.1.1`, `9.9.9.9`. Дополнительно в `/etc/gai.conf`
   добавлен `precedence ::ffff:0:0/96 100` — приоритет IPv4: у планшета
   нет IPv6-аплинка (IPv6 есть только в оверлее `fd00::/16`), и без этого
   curl/apt/pi сначала идут в AAAA и ждут таймаут.
2. **Группа `inet` (GID 3003).** На Aurora исходящая сеть разрешена
   процессам группы `inet`; в Debian такой группы нет, поэтому у НЕ-root
   пользователей chroot «не работал DNS» и вообще не открывались сокеты
   (`getent` пусто, `curl: 000`, `node: EAI_AGAIN`), хотя root ходил в
   сеть. Создана группа `inet` (3003), `user` и `clearssh` в неё
   добавлены — после этого у `user` работают DNS/HTTP/pi.
3. **`ping`.** Под root в chroot создание ICMP-сокета запрещено политикой
   Aurora (`ping: socket: Permission denied`) — файловые capabilities
   (`cap_net_raw`) не помогают, IPv4/IPv6 одинаково. Под `user` ping
   работает: unprivileged ICMP (ping socket, `net.ipv4.ping_group_range =
   0..2147483647`). Обёртка `/usr/local/bin/ping`
   (`scripts/ns220-chroot/ping-wrapper.sh`) от root перезапускает ping от
   `user` (uid 100000, gid inet) — `ping` работает в обоих случаях.
   Для `ya.ru` нужен `-4`: у него есть AAAA, а IPv6-аплинка нет.

## 17.10. pi: прокси и HyperQwen

* Окружение pi (`/etc/profile.d/90-llm-proxy.sh`, подключается из
  `/root/.bashrc`): `HTTP_PROXY`/`HTTPS_PROXY` =
  `http://user:12345678@127.0.0.1:1080`, `ALL_PROXY` =
  `socks5h://...`, `NO_PROXY` = localhost/127.0.0.1/::1 + LAN.
  Оверлей `fd00::/8` в NO_PROXY **не** входит: запросы к HyperQwen идут
  через cnet-прокси (CONNECT на IPv6-литерал), а тот — через node-de.
* Ключ HyperQwen: `~/.config/opencode/hyperqwen.key` (0600) — на него
  ссылается `apiKey: "!cat ~/.config/opencode/hyperqwen.key"` в
  `models.json` (провайдер `hyperqwen-cnet`, модель `qwen3.8-27b`).
* Проверено: `pi -p "..." --model qwen3.8-27b` отвечает через
  оверлей (через node-de); внешние провайдеры (speshu, modelgate)
  работают через тот же прокси.

## 17.11. Звук: сигнал pi (attention-sound)

* Задача: pi должен «пищать» на планшете, когда закончил ход. Расширение
  `~/.pi/agent/extensions/attention-sound.ts` по событию `agent_settled`
  проигрывает `beep.wav` внешним плеером, перебирая в PATH
  `paplay → pw-play → aplay → ffplay → mpv`.
* Что сделано: в chroot установлен `alsa-utils` и используется **aplay**.
  Устройство `/dev/snd` доступно (bind `/dev` из Aurora, карта
  `mt-snd-card`, device 0) — `aplay` работает и от root, и от uid 100000.
  Громкость — системная (кнопки громкости Aurora).
* Почему не PulseAudio: демон Aurora (`pulseaudio` от uid 100000) принимает
  по unix-сокету только клиентов своего uid, а pi работает от root —
  `paplay` получает «Access denied»/«Connection refused». Пакет
  `pulseaudio-utils` из chroot удалён, чтобы расширение не выбирало
  нерабочий paplay и сразу брало aplay.
* Файл сигнала вынесен в читаемое всем место:
  `/usr/local/share/pi/sounds/beep.wav`, переменная
  `PI_ATTENTION_SOUND` (в `/etc/profile.d/91-audio.sh`, подключается из
  `/root/.bashrc`).
* Проверено: `PI_ATTENTION_FORCE=1 PI_ATTENTION_DEBUG=1 pi -p ...` пишет
  `[attention-sound] playing /usr/local/share/pi/sounds/beep.wav x2 via aplay`
  и сигнал слышен; `aplay` от root и от uid 100000 — OK.

## 17.12. Известные ограничения и что дальше

* Нет systemd внутри chroot — сервисы держит `ns220-svc` на хосте.
  При перезагрузке планшета их поднимает `ns220-chroot.service`
  (проверено).
* `csshd` в chroot — отдельный сервер (порт 222); он не связан с
  Aurora SSH (порт 22).
* Пароли стенда: root в chroot `1234`, sudo у `user` без пароля —
  всё это dev-стенд, наружу не выставлять.
* Для pi на armv7 нет авто-загрузки `rg`/`fd` — держать пакеты
  Debian `ripgrep`/`fd-find` установленными.
* Проверить при обновлении Aurora: `debian-chroot` (перезапись при
  обновлении `dup340.sh`?) и `ns220-chroot.service` (systemd unit).

# Отчёт: планшет MVK-2020 / Aurora OS — работы 19.11.2026

Автор: ассистент (pi). Всё, что делали с устройством, где лежат образы/бэкапы и как продолжить.

---

## 0. TL;DR

* Обновили Aurora OS **4.0.2.249 → 4.0.2.340 (Balakovo)** (userland + hotfixes, 793 пакета), ядро осталось 3.18.79 (adaptation-слой 249 — закрытый репо OMP).
* Получили **root** (`devel-su`/`su`/`ssh root`, пароль `1234`), SSH по USB, 2 ГБ swap на eMMC + 1 ГБ zram, набор dev-утилит.
* Выяснили и подтвердили vendor-lockdown: **IMA appraisal (ima_appraise=enforce) + module.sig_enforce=1 + rpm validation plugin + secure boot (LK)** — любой не-ОМР бинарник не запускается, локально собранный RPM устанавливается только после подмены CA, но файлы всё равно не исполняются.
* Пытались снять IMA через boot-образ (cmdline в заголовке, патч сжатого ядра) — **все модифицированные образы не грузятся**, заводской образ грузится нормально.
* **Сейчас устройство окирпичено** (не включается/не видно на USB) — вероятно разряд или halt в LK. Разделы не повреждены, заводской boot-образ у нас есть. План восстановления — раздел 12.
* Все артефакты — на хосте (`~/aurora-ns220/...`) и на microSD внутри планшета (`/aurora-backup/...`).

---

## 1. Устройство

| Параметр | Значение |
|---|---|
| Модель | **MVK-2020** (Byterg), variant `aq_ns220`, плата `W1C9` (MT8735A/W), SoC MediaTek MT6735/MT8737 (ARMv7, 4 ядра) |
| Serial | `INS220067941` |
| RAM | 1 960 644 kB (~1.9 ГБ), swap не было изначально |
| Накопитель | eMMC 14.6 ГБ (`mmcblk0`), microSD 29.8 ГБ (`mmcblk1`) |
| Ядро | `Linux 3.18.79 #1 SMP PREEMPT Mon Mar 27 14:47:29 UTC 2023 armv7l` |
| ОС до | **Aurora 4.0.2.249 (Balakovo)**, ssu release 4.0.2.249 |
| ОС после | **Aurora 4.0.2.340 (Balakovo)**, `aurora-version-4.0.2-1.2.340.omp` |
| Пароли | `root` = `1234` (установлен нами), `devel-su` = `1234` (был), `defaultuser` = `1234` |

Разметка eMMC (by-name: `/dev/block/platform/mtk-msdc.0/by-name/`):

```
boot p8 (16M)      recovery p9 (16M)   lk p6 (512K)      logo p10 (8M)     items p5 (2M)
system p23 (2.5G)  vendor p22 (512M)   userdata p25 (11G) cache p24 (400M)
nvdata p19 (32M)   nvram p2 (5M)       metadata p21 (35M) expdb p11 (10M)
tee1 p16 / tee2 p17 (5M)  secro p14 (6M)  protect1 p3 / protect2 p4 (10M)
keystore p15 (8M)  frp p18 (1M)        para p7 (512K)     seccfg p12 (512K)
odmdtbo p20 (16M)  oemkeystore p13 (2M) flashinfo p26 (16M)
```

`userdata p25` содержит LVM: `sailfish-root` 2.5 ГБ → `/`, `sailfish-home` 8.5 ГБ → `/home`.

---

## 2. Доступ к устройству (как работали)

USB-режимы, которые мы видели:

| VID:PID | Что это | Комментарий |
|---|---|---|
| `0bb4:0a02` | ОС Aurora, dev-режим: RNDIS + CDC | хост получает `192.168.2.14/24`, планшет `192.168.2.15`; через ~60–90 с после старта |
| `0bb4:0afe` | LK boot-menu / UMS | `/dev/sda` размер 0 — это LK, а не система |
| `0e8d:2000` | MTK preloader | живёт ~2.5 с, отдаёт CDC ACM (`ttyACM0`); **mtkclient не может handshake** («Handshake failed / Sync error») |
| `0bb4:0c01` | fastboot LK | работает; `getvar`: `secure: yes`, `unlocked: no`, `product: TB8735AP1_BSP`, `max-download-size: 0x8000000` |

SSH (когда система поднята):

```bash
unset HTTP_PROXY HTTPS_PROXY           # в окружении хоста были прокси 127.0.0.1:1080 — не нужны
sshpass -p 1234 ssh root@192.168.2.15     # root (мы включили PermitRootLogin yes)
sshpass -p 1234 ssh defaultuser@192.168.2.15
```

Готовые помощники на хосте:

* `~/aurora-ns220/runroot <script.sh>` — scp скрипта на планшет + запуск через `devel-su` (root).
* `~/aurora-ns220/root_cmd.sh '<cmd>'` — выполнить команду от root.
* `~/aurora-ns220/dumpall.sh` — дамп всех разделов ≤20 МБ в `/sdcard/aurora-backup/partitions-all/`.

Инструменты на хосте: `fastboot` (apt, 34.0.5), `adb` (`~/.local/bin`), `mtkclient` (`/tmp/mtkclient-main`, pip `mtkclient 2.1.4`, бинарь `~/.local/bin/mtk`).

---

## 3. Что сделали с ОС

### 3.1 Разведка репозиториев (ssu)

`ssu lr` на устройстве (домен `sales`, brand OMP, variant aq_ns220):

* публичные (200): `omp`, `omp-apps`, `hotfixes/aq_ns220`
* закрытые: `omp-hw/adaptation-mediatek-aq_ns220` → **401**, `pj:hw:common` → **401**, `pj:apps` → 404/401
* `features/…/docviewer`, `sailfish-eas` → 404

GitHub-подобный листинг доступен: `https://releases.omprussia.ru/` (media/, releases/, README.txt).
Найденные релизы: `4.0.2.209`, `4.0.2.249`, `4.0.2.303`, `4.0.2.310`, **`4.0.2.340`** (последний публичный; 341–420 отсутствуют).
`droid-hal-aq_ns220-img-boot` в публичных `omp`-репозиториях **нет** (только в закрытом adaptation) — поэтому заводской boot мы брали из бэкапа, а не из репо.

### 3.2 Обновление 249 → 340

```bash
# на устройстве, root
ssu disablerepo adaptation0 apps docviewer sailfish-eas
ssu re 4.0.2.340
# zypper поставили из 4.0.2.249 (apt-подобного нет): zypper, augeas-libs, readline
zypper -n --no-gpg-checks dup --download-in-advance
```

* Сначала установили `zypper` (из репо 249), затем переключили release на 340.
* Заблокировали 32 пакета, которые физически недоступны (hw/jolla apps):
  `audiosystem-passthrough, audiosystem-passthrough-dummy-hw2_0, bluebinder, geoclue-provider-hybris-binder, gmp-droid, gstreamer1.0-droid, hybris-libsensorfw-qt5-binder, libgbinder, libgbinder-radio, libgrilio-binder, libncicore, libnciplugin, nfcd-binder-plugin, oem-unlockable-service, ofono-ril-binder-mtk-plugin, ofono-ril-binder-plugin, ofono-ril-plugin, qtscenegraph-adaptation, sailfish-connman-plugin-suspend, sailfish-connman-plugin-tethering-wmtwifi, streamcamera-droid-plugin, jolla-calculator, jolla-calendar, jolla-email, jolla-mediaplayer, jolla-notes, libjollamediaplayer, logviewer, omp-weather, sailfish-audiorecorder, sailfish-components-calendar-qt5, sailfish-filemanager`
  (`zypper al <pkg>`; список смотреть `zypper ll`)
* Результат: **793 пакета обновлено**, 237 МБ загрузки, `aurora-version-4.0.2-1.2.340.omp.noarch`.
* Проблемы и решения по ходу:
  * PackageKit (packagekitd) держал zypp-lock → `systemctl stop packagekit`, `systemctl mask packagekit`, `mv /usr/share/dbus-1/system-services/org.freedesktop.PackageKit.service /root/…disabled`, `pkill -f packagekitd`. **Потом вернули обратно** (unmask + файл на место + start).
  * Битый кэш (`readline ... поврежден при передаче`) → `zypper clean -a` и заново.
  * `systemd-run --unit=…` использовали, чтобы процесс не убивался при обрыве SSH.
  * `zypper` вернул 107 = «нужен ребут» (это нормально).
* После ребута: `4.0.2.340`, swap/zram поднялись, сервисы живы, единственный failed unit — `quota@home.service` (не критично).

### 3.3 Что важно помнить про ssu/zypper

* Смена релиза: `ssu re 4.0.2.340` (обратно — `ssu re 4.0.2.249`).
* `ssu updaterepos` / `zypper ref` — обновить метаданные; `ssu up` требует регистрации устройства (не зарегистрировано).
* `gpgcheck=0` в репозиториях (можно ставить неподписанные rpm, но работает rpm-validation plugin — см. раздел 7).

---

## 4. Root, SSH, пользователи

* `devel-su` (setuid root) с паролем `1234` — работал изначально.
* Установили пароль root: `passwd root` → `1234`; раньше root был заблокирован (`root:!`).
* Включили `PermitRootLogin yes` в `/etc/ssh/sshd_config` (бэкап: `/etc/ssh/sshd_config.bak-<дата>`), `systemctl restart sshd`.
* Проверено: `ssh root@192.168.2.15` и `echo 1234 | su -c id` → `uid=0`.
* **Переименование `defaultuser` → `user` НЕ выполнялось.** Пользователь просил («если возможно и не противоречит»). Оценка: технически возможно (`usermod -l user -d /home/user -m defaultuser`, `groupmod -n user defaultuser`, правка `/etc/group`, `/etc/gshadow`, `/etc/subuid`, `/etc/subgid`, `/etc/connman/connman-vpn.conf`), но Aurora/ Sailfish местами ожидает именно `defaultuser`, плюс возможны проблемы с сессией и обновлениями. Делать только после стабилизации системы и с бэкапом `/home` и `/etc`.
* `grep -rl defaultuser /etc` → `/etc/connman/connman-vpn.conf`, `/etc/subuid`, `/etc/subgid`, `passwd/shadow/group` (+ их `-` версии).

---

## 5. Память/своп/мониторинг

* **swapfile 2 ГБ**: `/home/swapfile` (LVM на eMMC, запись ~48 МБ/с), `pri=10`.
  Unit: `/etc/systemd/system/swapfile.service` (enabled, `RequiresMountsFor=/home`).
* **zram 1 ГБ** (lz4, `pri=100`): unit `/etc/systemd/system/zram-swap.service` (enabled):
  `echo lz4 > /sys/block/zram0/comp_algorithm; echo 1073741824 > /sys/block/zram0/disksize; mkswap /dev/zram0; swapon -p 100 /dev/zram0`.
  zram **встроен в ядро** (модуль не нужен), доступные алгоритмы: `lzo [lz4] lz4k`.
* `vm.swappiness=30` → `/etc/sysctl.d/60-swappiness.conf`.
* Проверено после перезагрузки: оба swap на месте (2048 + 1024 МБ).
* **`/usr/bin/sysmon`** — скрипт мониторинга (RAM, top CPU/RAM, диск, термозоны mtkts*, I/O mmcblk, swap).
  ⚠️ Из-за IMA запускать **через интерпретатор**: `sh /usr/bin/sysmon` (иначе `Permission denied`).

---

## 6. Установленные пакеты

`git 2.26.2`, `gcc`, `make`, `rpm-build`, `rsync`, `openssh-server/-clients`, `python3 3.8.11`, `htop`, `nano`, `screen`, `tcpdump`, `strace`, `openssl 1.1.1o`, `fingerterm 1.4.5`, `keyutils-libs`, `augeas-libs`, `readline`, `findutils(busybox symlink)`, `zypper 1.14.38`.

**Нет в репозитории Aurora:** `nmap`, `mc`, `ncdu`, `vim`, `tmux`, `qemu(-user)`, GNU `findutils`, X11/Wayland-чужие клиенты.

Тонкости сборки rpm на устройстве:

* `rpmbuild` падал с `Could not canonicalize hostname` → сделали `/etc/hostname` = `MVK-2020.localdomain` и записи в `/etc/hosts` (`127.0.0.1 localhost.localdomain localhost`, `127.0.1.1 MVK-2020.localdomain MVK-2020`).
* `brp-strip-static-archive` падает (busybox `xargs` без `-d`) → собирать с `--define "__brp_strip_static_archive /bin/true"`.

---

## 7. Защита исполнения (главный блокер)

### 7.1 Факты

* `/proc/cmdline` (рабочая загрузка):

```
console=tty0 root=/dev/ram vmalloc=496M androidboot.hardware=mt6735 slub_max_order=0 slub_debug=OFZPU
firmware_class.path=/vendor/firmware androidboot.verifiedbootstate=green bootopt=64S3,32N2,32N2 selinux=0
ima_appraise=enforce ima_hash=sha256 ima_tcb module.sig_enforce=1 clearmem=on security=diehard
ddr_name=H9CKNNNBJTMPLR_NUH ddr_speed=1066000 androidboot.veritymode=enforcing androidboot.atm=disabled
androidboot.meta_log_disable=0 printk.disable_uart=1 bootprof.pl_t=4448 bootprof.lk_t=2088
bootprof.logo_t=1582 boot_reason=4 androidboot.serialno=INS220067941 androidboot.bootreason=wdt_by_pass_pwk gpt=1
```

* IMA: `/sys/kernel/security/ima/` содержит `binary_runtime_measurements`, `ascii_runtime_measurements`, `runtime_measurements_count`, `violations`. **Файла `policy` нет** → политику/аппрайз в runtime не поменять.
* Экспериментально: `execve` любого нашего файла → `EACCES` (в /tmp, /vendor_data, /home, /usr/bin); `dlopen` своего `.so` из python3 → `failed to map segment` (MMAP-аппрайз).
* Копия подписанного бинарника с сохранением xattr (`cp -a /usr/bin/git /tmp/git`) — **работает** (`git --version`). Т.е. решает наличие `security.ima` и доверенного ключа.
* `.ima` keyring: 2 ключа; loader — `/usr/libexec/ima/load-certificates` (systemd `ima-certificates-loader.service`) берёт сертификаты из БД `/var/lib/rpmsign-external/db.sqlite` и делает `add_key(".ima", …)`.
  Наш самоподписанный сертификат: **`Could not load certificate (747cac19) to the .ima keyring`** — ядро разрешает только ключи, подписанные доверенным (OMP) ключом.
* rpm-validation plugin (`/usr/lib/rpm-plugins/validation.so`, конфиг `/usr/share/rpm-validator/configuration.json`, хуки `tsm_pre/tsm_post`):
  требует CMS-подпись «developer», цепочку к `/etc/keys/packages/ca.pem` и keyid == `/etc/keys/packages/system-developer-keyid` (`e2b3d011…`).
  Ошибка: `hello-test does not have developer signature` / `certificate verify error: unable to get local issuer certificate`.
* Что мы сделали и что получилось:
  * `rpmsign-external makecertca/makecert/sign` — собрали свою CA и dev-cert, подписали тестовый rpm. ✔️
  * Добавили свою CA в конец `/etc/keys/packages/ca.pem` (бэкап: `/etc/keys/packages/ca.pem.orig-omp`) → валидация проходит, пакет ставится. ✔️
  * Файлу проставляется `security.ima` — но подписан **нашим** ключом (`747cac19`), которого нет и нельзя добавить в `.ima` keyring → **запуск всё равно `EACCES`**. ✖️
* Также: `rpm -i --define '__transaction_validation %{nil}'` отключает валидацию (устанавливает без подписи), но файлы тогда вообще без `security.ima` → тоже не запускаются.
* В ядре есть `security=diehard`, но **строки/символа `diehard` в ядре нет** — это неизвестный параметр (игнорируется). LSM diehard не участвует.

### 7.2 Вывод

Без **ключей OMP** (developer-сертификат, доверенный в `.ima`) native-бинарники (C/C++/Go/Rust) на этой прошивке не запустить. Интерпретируемый код (python3, bash-скрипты через `sh`) работает, т.к. интерпретатор подписан.

---

## 8. Эксперименты с boot-образом (хронология)

Формат заводского boot (`boot-stock.img`, 12 331 008 байт, md5 `d915a46fd05d2472f1ba1c84e65e86da` = `/boot/hybris-boot.img`):

```
0x0000  ANDROID! header (page_size = 2048; cmdline в заголовке — пустой)
        kernel_size = 8 066 647, ramdisk_size = 4 261 699
0x0800  zImage: magic 0x016f2818 @0x24, start=0, end=0x7a1ef8
        stub 0x0000…0x4758, gzip-payload 0x4758…0x7a1ef8 (7 985 056 байт)
        DTB сразу после: 0x7a1ef8, totalsize 63 327, конец 0x7b1657 (= kernel_size)
        (внутри распакованного Image тоже есть DTB-магия по 0x264 — не помеха)
ramdisk: 4 261 699 байт, выровнен на 2048
```

Распаковка ядра: `zlib.decompressobj(31)` от 0x4758 → Image **20 860 928** байт.
Строк `ima_appraise=enforce` в Image **нет** (только таблица параметров: `ima_appraise=`, `ima_tcb`, `module.sig_enforce`).

| Образ | md5 | Что внутри | Результат |
|---|---|---|---|
| `boot-stock.img` | `d915a46f…` | заводской (12.33 МБ) | — (эталон) |
| `boot-stock-16m.img` | `35c87139…` | заводской + нули до 16 МБ | ✅ **грузится** (проверено дважды) |
| `boot-ima-off.img` | `9aae1100…` | cmdline в заголовке = полный набор + `ima_appraise=off module.sig_enforce=0` | ❌ boot-loop |
| `boot-t1.img` | `86eaee31…` | cmdline в заголовке = только `ima_appraise=off` | ❌ boot-loop |
| `boot-imaoff-v2.img` | `7d7bd25e…` | патч в распакованном Image: `ima_appraise=`→`ima_qppraise=`, `module.sig_enforce`→`module.xig_enforce`; zopfli-gzip + нулевой паддинг до 0x7a1ef8 | ❌ boot-loop |
| `boot-imaoff-v3.img` | `025c6143…` | тот же патч, gzip -9, **DTB сдвинут**, обновлены `zImage.end` (0x7a3269) и `kernel_size` (8 071 624) | ❌ boot-loop (последний залитый) |
| `boot-with-cmdline.img` | — | первый вариант (на SD) | ❌ |

Выводы:

* `fastboot flash boot` **разрешён** даже при `secure: yes / unlocked: no` (Sending/Writing OKAY) — так же работает `dd` в `/dev/block/platform/mtk-msdc.0/by-name/boot` из-под root.
* Любое изменение содержимого образа → не грузится; заводское содержимое (даже с нулями вместо «мусора» в хвосте раздела) → грузится.
* Проверка гипотез: хэша заводского образа нет ни в `lk/para/seccfg/items/protect1/protect2/flashinfo/nvdata/odmdtbo/keystore/frp/oemkeystore`; `secro`, `para`, `seccfg` — целиком нули.
* Скорее всего LK всё же верифицирует boot (secure boot `SBOOT`, но конфиг в secro пустой → неясно), **либо** наш патч ядра ломает раннюю инициализацию (IMA-политика/аппрайз).
* Разблокировка: `fastboot oem unlock` → на экране «unlock failed», `unlocked: no` остаётся. После этой команды LK иногда залипает в состоянии «Start unlock flow» и перестаёт отвечать (лечится полным перезаходом в fastboot).

---

## 9. Наблюдаемое поведение (пригодится для диагностики)

* Неудачный образ: цикл ~10–60 с, на USB периодически мелькает `0e8d:2000`.
* Заводской образ: через ~60–90 с появляется `0bb4:0a02`, хост получает `192.168.2.14`, SSH на `192.168.2.15` работает (root/1234).
* Если планшет стоит в boot-меню: `0bb4:0afe`, `/dev/sda` 0 байт (LK UMS).
* `printk.disable_uart=1` — последовательного лога ядра нет; preloader-CDC (`ttyACM0`) отдаёт только свой протокол (mtkclient: «Sync error»).

---

## 10. Где что лежит (артефакты)

### Хост: `/home/user/aurora-ns220/`

```
bootdump/   boot-stock.img (12.33M, md5 d915a46f…)     ← ЭТАЛОН, им восстанавливать
            boot-stock-16m.img (16M, md5 35c87139…)     ← этим флешить
            boot-ima-off.img, boot-t1.img, boot-imaoff-v2.img, boot-imaoff-v3.img (эксперименты)
            lk.img (md5 9b26b96e…), para.img (59071590…), seccfg.img (59071590…), items.img (dcfe6432…)
parts/      flashinfo, frp, keystore, nvdata, odmdtbo, oemkeystore, protect1, protect2
meta/       omp.primary.xml, omp-apps.primary.xml (340), hotfixes, installed.txt, updates.json
bin/        validation.so, external_ima.so, load-certificates, update-revocation-list (анализ)
lk/         lk_signoff.bin (+ lk_signoff.zip в корне)
fw/         p249.xml, repomd249.xml (метаданные 249)
runroot, root_cmd.sh, dumpall.sh — скрипты доступа
LINUX-ON-NS220-NOTES.txt (старые заметки)
```

### microSD (внутри планшета), `/aurora-backup/`

```
partitions/       boot-current.img (ИСХОДНЫЙ дамп 16M; ⚠️ при чтении с кардридера давал I/O error),
                  boot-with-cmdline.img, boot-stock-16m.img, boot-ima-off.img,
                  lk, logo, para, proinfo, seccfg, recovery, expdb, metadata, nvram, tee1, tee2
partitions-all/   25 образов ≤20 МБ (boot, cache?, expdb, flashinfo, frp, items, keystore, lk, logo,
                  metadata, nvdata, nvram, odmdtbo, oemkeystore, para, proinfo, protect1, protect2,
                  recovery, seccfg, secro, tee1, tee2, …)
config/           boot/ (hybris-boot.img, hybris-recovery.img, lk.img.gz, logo.bin.gz, odmdtbo.img.gz,
                  MT6737_Aurora_OS_scatter.txt, items.ini), ssu.ini, rpm-qa-before.txt,
                  os-release-before.txt, db/ (копия /var/lib/rpm)
логи:             download.*.log, download.history, dup.sh, dl.sh
```

### На планшете (когда поднимется)

```
/home/defaultuser/boot-stock-16m.img, boot-t1.img, boot-imaoff-v2.img
/etc/systemd/system/swapfile.service, zram-swap.service
/etc/sysctl.d/60-swappiness.conf
/usr/bin/sysmon
/etc/keys/packages/ca.pem.orig-omp   (наш CA дописан в ca.pem — при желании вернуть)
/etc/ssh/sshd_config.bak-<дата>
/root/org.freedesktop.PackageKit.service.disabled  (если снова пригодится — вернуть на место)
/usr/bin/hello-test (тестовый rpm), /root/sign/ (наши CA/ключи/подписанный тестовый rpm)
/tmp/bt (сборка тестового rpm)
```

---

## 11. Текущее состояние (на момент отчёта)

* Последний залитый boot — **`boot-imaoff-v3.img` (нерабочий)** → устройство ушло в boot-loop, затем **перестало определяться на USB и не включается** (по словам пользователя).
* Разделы при этом не повреждены: мы меняли только `boot` (перезаписываемо), `recovery/secro/seccfg/para/metadata/…` не трогали.
* Вероятные причины «кирпича»: разряд аккумулятора (долгие циклы с подсветкой) или halt в LK после N неудачных загрузок. Обычно лечится зарядкой + сбросом питания.

---

## 12. План восстановления

1. **Зарядить** 30–60 минут от USB/зарядки (даже если экран чёрный).
2. Сброс питания: держать **Power 20–30 секунд**, отпустить.
3. Вход в boot-меню: зажать **Volume Down** и подать питание (или вставить USB). Появится меню
   `recovery / fastboot / normal` → выбрать **fastboot** (Volume Down — навигация, Power — выбор).
4. Проверить и залить заводской образ:

```bash
unset HTTP_PROXY HTTPS_PROXY
fastboot devices
fastboot flash boot ~/aurora-ns220/bootdump/boot-stock-16m.img
fastboot reboot
```

   Проверено дважды: после этого система загружается, через 1–2 минуты — SSH на `192.168.2.15`.

5. Если fastboot недоступен:
   * `0e8d:2000` (preloader, ~2.5 с): `mtk printgpt` в цикле — исторически **не** успевает handshake;
   * `0e8d:0003` (BROM) — кнопками вызвать не удалось; варианты: удерживать **Vol Up + Vol Down** при подаче питания, либо тест-поинты платы (не делали).
6. После загрузки — включить dev-режим/SSH (Настройки → Для разработчиков; логин `root`/`1234`).

### Если система поднялась — быстрая самопроверка

```bash
sshpass -p 1234 ssh root@192.168.2.15 'cat /etc/os-release | head -3; cat /proc/swaps; systemctl --failed --no-pager'
sh /usr/bin/sysmon                     # на устройстве
```

---

## 13. Куда двигаться по задаче «максимальные возможности»

1. **`sdk-deploy-rpm`** (`/usr/bin/sdk-deploy-rpm` — есть в системе!) — официальный способ доставки пакетов из Aurora SDK. Гипотеза: он умеет регистрировать developer-сертификат в `.ima` keyring через привилегированный сервис → тогда native-код заработает **без** взлома boot. Проверить первым делом.
2. **`rnd-dist-upgrade`** (`/usr/bin/rnd-dist-upgrade`, входит в `jolla-developer-mode`) — переключение на RnD-репозитории домена `community` (`os-repo.omprussia.ru`). Возможно, там доступны dev-сборки/ключи/полные наборы пакетов.
3. Официальный путь: **developer-сертификат OMP** (Aurora OS developer program) → `rpmsign-external` → установка и исполнение подписанных пакетов штатно.
4. Пока IMA не снят: разработка на **python3**/bash (интерпретаторы подписаны), сборка бинарников «на будущее» без запуска.
5. Debian chroot (для go/rust/gcc-свежих) станет возможен **только после** снятия IMA — иначе в chroot ничего не запустится.
6. Если когда-нибудь появится возможность менять cmdline/LK: цель — убрать `ima_appraise=enforce` и `module.sig_enforce=1`. Осторожно: LK-патч = риск кирпича без fastboot (fastboot живёт в LK!), kernel-патч = требуют решения проблемы запуска модифицированного zImage.

---

## 14. Точные данные для продолжения

* `fastboot getvar all` (важное): `max-download-size: 0x8000000`, `partition-type:boot: raw data`, `unlocked: no`, `secure: yes`, `product: TB8735AP1_BSP`, `version-preloader: 0.1.00`, `version: 0.5`.
* Разметка разделов — раздел 1; scatter-файл: `/boot/MT6737_Aurora_OS_scatter.txt`.
* Boot-образ: kernel `8 066 647`, ramdisk `4 261 699`, page `2048`, zImage gzip-payload `0x4758…0x7a1ef8`, DTB `63 327` с `0x7a1ef8`.
* `ssu`-репозитории (4.0.2.249/340): `omp`, `hotfixes/aq_ns220`, `omp-apps` — публичные; `omp-hw/adaptation-mediatek-aq_ns220` — 401.
* Термозоны: `mtktscpu`, `mtktsbattery`, `mtkts1..3`, `mtktsAP`, `mtkwmt`, `mtktsbtsmdpa`.
* Быстрый доступ к Wi-Fi (если RNDIS не поднимается): Wi-Fi планшета был в сети `192.168.1.0/24` (host `192.168.1.198`); SSH-порт 22.
* В окружении хоста **были** `HTTP_PROXY/HTTPS_PROXY=http://127.0.0.1:1080` — для сетевых команд их надо `unset` (пользователь прокси не использует).

---

## 15. Мелкие факты/грабли (чтобы не повторять)

* `aurora-version` определяет содержимое `/etc/os-release` → после dup версия берётся из пакета, а не из ssu.ini.
* `zypper dup` в фоне запускать только через `systemd-run` (иначе обрыв SSH убивает процесс).
* PackageKit, стартовавший по DBus, снова захватывает zypp-lock → на время zypper надо маскировать `packagekit.service` и убирать `org.freedesktop.PackageKit.service` из `/usr/share/dbus-1/system-services/`.
* `rpmbuild` на устройстве: нужен `/etc/hostname`-FQDN и `--define "__brp_strip_static_archive /bin/true"`.
* Свои скрипты/бинарники: `exec` запрещён IMA; скрипты запускать `sh script`, а не `./script`.
* Каталог `/home/user/aurora` на хосте в середине сессии неожиданно исчез (восстановлен вручную) — если что-то пропало, проверять.
* Устройство «залипает» в LK-меню: USB = `0bb4:0afe`, `/dev/sda` 0 байт — это **не** система, надо выбрать `normal`.

---

## 16. Резюме по целям пользователя

| Цель | Статус |
|---|---|
| 1. Обновить ОС до последней поддерживаемой | ✅ 4.0.2.340 (userland). ⚠️ adaptation/ядро — 249 (закрытый репо). |
| 2. Root | ✅ `devel-su`/`su`/SSH root, пароль `1234`. |
| 3. Репозитории свободного/российского ПО | ⚠️ Частично: публичные OMP-репо; сообществ/OpenRepos для armv7hl Aurora не подключали (риск ABI + IMA всё равно блокирует запуск). |
| 4. Среда разработки на устройстве | ❌ Блокер: IMA + rpm-validation + secure boot. Установлены gcc/make/git/python3/rpm-build, но собранное не запускается. Следующий шаг — `sdk-deploy-rpm` / RnD / сертификат OMP. |
| 5. Полезные программы | ⚠️ Частично: git, gcc, make, rsync, python3, htop, nano, screen, tcpdump, strace, openssl, fingerterm. Нет в репо: nmap, mc, ncdu, vim, tmux. |
| Swap 2 ГБ + мониторинг | ✅ swap 2 ГБ (eMMC) + zram 1 ГБ, `htop`, `sysmon`. |
| Переименовать defaultuser → user | ⏳ Не делали (возможно, но рискованно; см. раздел 4). |

*Отчёт создан 2026-11-19. Все md5/пути проверяемы по файлам на хосте и microSD.*

---

## 17. Дополнение (после отчёта): планшет не включается вообще

Состояние на момент дополнения (19.11.2026, ~17:00):

* На USB **не определяется** (`lsusb` пусто: ни `0bb4:*`, ни `0e8d:*`).
* На кнопку **Power** (в т.ч. удержание 20–30 с) — реакции нет.
* На кнопку **Reset** — реакции нет.
* Кардридер с microSD отключён (сама карта с бэкапами — внутри планшета).

### Что это может быть (по вероятности)

1. **Глубокий разряд аккумулятора.** За сегодня: ~10 полных циклов загрузки/перезагрузки, долгие boot-loop'ы с подсветкой и USB-питанием, zypper-dup с интенсивным I/O. Для MTK-планшетов типично: при глубоком разряде контроллер не даёт включаться, пока напряжение не поднимется (иногда 30–120 минут на зарядке), и USB-порт ПК (0.5 А) для этого может быть недостаточно.
2. **Halt/защита в LK** после серии неудачных загрузок (в LK есть `battery_protect_loop`, `LOW_POWER_OFF_CHARGING_BOOT` — при низком заряде он уходит в режим зарядки без USB-энумерации).
3. Аппаратная проблема (маловероятно): мы писали только раздел `boot` через `dd`/`fastboot`, остальные разделы (preloader, lk, secro, recovery, seccfg, para) не изменялись. Программно «убить» preloader мы не могли — он не перезаписывался.

**Данные/разделы целы.** Даже если устройство сейчас не стартует, восстановление — вопрос питания и доступа к fastboot, а не потери данных.

### Что делать (по шагам)

1. **Зарядка от мощного источника** (не от USB-порта ПК): оригинальный/2 А адаптер, 60–120 минут. Не дёргать кнопки в это время.
2. Проверить признаки жизни: нагрев, LED, реакция экрана при коротком нажатии Power.
3. После зарядки:
   * короткое нажатие **Power** (3 с) — обычное включение;
   * если нет — **Power 30 с** (сброс), затем снова Power;
   * если нет — зажать **Volume Down**, подать питание → boot menu → `fastboot`.
4. В fastboot сразу залить заводской образ:

```bash
unset HTTP_PROXY HTTPS_PROXY
fastboot devices
fastboot flash boot ~/aurora-ns220/bootdump/boot-stock-16m.img
fastboot reboot
```

5. Если fastboot не появится, но устройство будет мелькать как `0e8d:2000`:
   * попробовать поймать preloader mtkclient'ом: `cd /tmp/mtkclient-main && python3 mtk.py printgpt` (в цикле);
   * BROM (`0e8d:0003`) кнопками вызвать не удалось — остаются тест-поинты на плате (искать `KCOL0`/`EINT`/`BROM` пины; **не делали**), либо сервисный кабель.
6. После восстановления: включить dev-режим и SSH (`root`/`1234`), затем вернуться к разделу 13 (sdk-deploy-rpm → RnD → сертификат OMP).

### Важно для продолжения

* Не нужно заново делать дампы/бэкапы — всё уже есть (разделы 10).
* Единственная необратимая потеря на данный момент — ничего: система на eMMC, бэкапы на microSD и на хосте.
* Если планшет оживёт, **первым действием** — `fastboot flash boot boot-stock-16m.img` (не эксперименты).

---

## 18. ВОССТАНОВЛЕНИЕ УДАЛОСЬ — рабочий метод прошивки через CDC preloader (проверено)

Планшет снова грузится: **Aurora 4.0.2.340 (Balakovo)**, kernel 3.18.79, swap 2 ГБ + zram 1 ГБ, `boot` = заводской образ (md5 `35c87139ccc8e1a411aeb3ad4380688c`), failed units — нет.

### Что оказалось главным

Preloader этого планшета отдаёт **CDC-ACM (`/dev/ttyACM0`)** и говорит на **стандартном MTK preloader-протоколе**:
handshake `A0 0A 50 05` → устройство отвечает `READY` + `5F F5 AF FA`, дальше — обычные команды (`0xFD GET_HW_CODE` → hwcode `0x321` = MT6735, `target config 0x0` — SBC/SLA/DAA **выключены**, устройство незащищённое).

`mtkclient` по USB его не видит (ждёт bulk-интерфейс), а по serial не успевает из-за таймингов. Рабочее решение — **pty-мост**.

### Компоненты (лежат в /tmp, стоит сохранить)

1. **`/tmp/mtk_bridge7.py`** — неблокирующий мост:
   * создаёт pty и ссылку `/tmp/ttyMTK`;
   * **сам** делает handshake с реальным `/dev/ttyACM*` (мгновенно при появлении, что критично: окно preloader'а ~2.5 с);
   * держит сессию (пока порт открыт, preloader не сбрасывается);
   * переливает байты в обе стороны через очереди (без блокировок и потерь).
2. **`/tmp/mtkrun.sh <args>`** — стартует мост, ждёт `BRIDGE HS OK`, запускает `python3 mtk.py <args> --serialport /tmp/ttyMTK`, гасит мост.
3. Патчи в `/tmp/mtkclient-main` (все нужные уже внесены):
   * `Library/Port.py`: `run_serial_handshake()` заменён на «passthrough» (handshake делает мост); `echo()` терпимее; `wait_for_device` с интервалом 0.03;
   * `Library/Connection/seriallib.py`: быстрый `detectdevices()` через glob (+sysfs), минимальный таймаут чтения 0.6 с, ретраи для коротких ответов;
   * `Library/Connection/devicehandler.py`: `rdword()/rword()` с ретраями до 1.5 с;
   * `Library/DA/legacy/dalegacy_lib.py`: пауза 1.2 с после загрузки stage2 (DA инициализирует eMMC), запись чанками **0x1000 (4 КБ)** с паузой 4 мс и ожиданием ACK до 5 с.
4. **`/tmp/part_write.py`** — резервный инструмент: запись раздела напрямую через `0x70 SEND_PARTITION_DATA` (не использовался, но проверен на echo).

### Команда восстановления (как это сработало)

```bash
# 1) сбросить питание планшета (отключить USB, Power 40-60 с, подключить USB)
# 2) планшет уходит в цикл preloader (в dmesg мелькает 0e8d:2000 / ttyACM*)
/tmp/mtkrun.sh w boot /home/user/aurora-ns220/bootdump/boot-stock-16m.img --skipwdt
```

Лог успешной прошивки (сокращённо):

```
Handshake successful / HW code 0x321 / Device is unprotected / Device is in Preloader-Mode
DALegacy - Uploading legacy stage 1 from MTK_DA_V5.bin
Preloader - Jumping to 0x200000: ok.
DALegacy - Got loader sync! / Reading emmc info / ACK: 04029b
DALegacy - Uploading stage 2... Successfully uploaded stage 2
DALegacy - Connected to stage2
m_emmc_ua_size = 0x3a3e00000        (14.6 ГБ)
m_emmc_cid = 4a54443415010041dcf8c63952068bf7
Main - Handling da commands ...
Wrote boot-stock-16m.img to sector 64512 with sector count 32768.
```

После этого — Power 20–30 с, обычное включение → загрузился.

### Важные выводы на будущее

* **Флешинг этого планшета полностью восстановим**: DA грузится через preloader, а preloader живёт до LK. Значит можно экспериментировать даже с LK (сломали LK → перепрошили через preloader/DA).
* Если сессия сорвалась **до** запуска DA, preloader иногда «зависает» (перестаёт читать порт, не сбрасывается) — лечится только сбросом питания (Power 40–60 с).
* Не отправлять `0x70` (SEND_PARTITION_DATA) без полного пакета — preloader зависает, ожидая остаток.
* `--skipwdt` обязателен (иначе mtkclient отключает watchdog и зависание становится вечным).
* Запись через DA: чанк ≤16 КБ (в идеале 4 КБ), ожидание ACK до 5 с — иначе `Data ack failed for sdmmc_write_data`.
* Полезные проверки после восстановления:
  `md5sum /dev/block/platform/mtk-msdc.0/by-name/boot` (должен быть `35c87139ccc8e1a411aeb3ad4380688c` для заводского).

### Статус задачи «отключить проверку подписи»

Осталось нерешённым: `ima_appraise=enforce` + `module.sig_enforce=1` в cmdline (источник параметров так и не найден: не LK, не boot-header, не DTB в boot, не ranges/odmdtbo/items/para/seccfg/secro/metadata/nvram/nvdata/flashinfo/protect1-2; подозрение — **preloader**, его можно сдампить через DA).
Теперь эксперименты безопасны: любой неудачный образ откатывается `mtkrun.sh w boot boot-stock-16m.img --skipwdt`.

Проверенная гипотеза для следующего шага: изменить **один безвредный байт** в заголовке boot-образа (например, поле `name` вместо cmdline). Если планшет загрузится — значит LK образ не верифицирует, и проблема только в семантике наших правок; если нет — LK проверяет целостность образа, и нужен другой подход (патч LK / dump preloader / RnD-режим).

---

## 19. IMA ОТКЛЮЧЕН — найдена настоящая причина всех неудач с boot

### Причина

Я патчил cmdline boot-образа по смещению **0x38**, а в формате `ANDROID!` cmdline находится по **0x40** (0x30…0x3F — поле `name[16]`). Запись по 0x38 затирала вторую половину `name` и **сдвигала всю строку на 8 байт**, обрезая `bootopt=` → LK/ядро получали битый cmdline → загрузки не было. Отсюда все «boot-loop» ранее.

### Где лежали параметры

В командной строке **заголовка boot-образа** (0x40, 127 байт):

```
bootopt=64S3,32N2,32N2 selinux=0 ima_appraise=enforce ima_hash=sha256 ima_tcb module.sig_enforce=1 clearmem=on security=diehard
```

Именно поэтому их не было ни в LK, ни в разделах, ни в DTB — искать надо было в заголовке.

### Рабочий образ

`~/aurora-ns220/bootdump/boot-imaoff-v4.img` (16 МБ, md5 `0ff5bf5bea5e7d8396d1de1cd4467de8`):
единственное изменение — в cmdline заголовка (0x40):

```
bootopt=64S3,32N2,32N2 selinux=0 ima_appraise=off ima_hash=sha256 ima_tcb module.sig_enforce=0 clearmem=on security=diehard
```

Всё остальное (kernel_size, ramdisk_size, name, id, kernel, ramdisk, DTB) — байт в байт как заводское.

### Результат (проверено)

* `/proc/cmdline`: `ima_appraise=off`, `module.sig_enforce=0`
* `UNSIGNED_SCRIPT_OK` — свой shell-скрипт запускается напрямую
* `UNSIGNED_BINARY_OK` — **собранный gcc бинарник запускается** (gcc → ./hello работает)
* `DLOPEN_OK 7` — `dlopen` неподписанного `.so` из python3 работает
* Aurora 4.0.2.340, UI (lipstick) работает, swap/zram на месте, failed units — 0

**Итог: теперь на планшете полноценная нативная среда разработки (C/C++ и др.), без подписи OMP.**

Замечание: при загрузке LK один раз показал boot-меню (`0bb4:0afe`) — после выбора `normal` система загрузилась; на рабочем образе это разовое поведение.

---

## 20. Переименование пользователя: defaultuser → user (сделано)

* Сделан бэкап: `/root/rename-backup/` (`passwd`, `shadow`, `group`, `gshadow`, `subuid`, `subgid`, `home-config.tar.gz` ~131 МБ).
* `usermod -l` отказывался («user is currently used by process»), поэтому:
  1. `systemctl stop user@100000.service`
  2. `mv /home/defaultuser /home/user`
  3. `sed -i 's/\bdefaultuser\b/user/g'` по `/etc/passwd /etc/shadow /etc/group /etc/gshadow /etc/subuid /etc/subgid /etc/connman/connman-vpn.conf`
* **UID/GID не менялись** (100000) → права на файлы, квоты и LVM не затронуты.
* После перезагрузки проверено:
  * `getent passwd 100000` → `user:x:100000:100000:...:/home/user:/bin/bash`
  * `id` → `uid=100000(user) gid=100000(user)` + все группы на месте
  * `HOME=/home/user`, файлы (301 МБ) на месте, `lipstick` и `sailfish-homescreen` работают
  * `su - user -c 'devel-su id'` → root (пароль 1234)
  * `user@100000.service` активен, failed units — 0

Откат при необходимости: восстановить файлы из `/root/rename-backup/` и вернуть каталог `mv /home/user /home/defaultuser`.

---

## 21. Актуальное состояние (после всего)

| Параметр | Значение |
|---|---|
| ОС | Aurora 4.0.2.340 (Balakovo), kernel 3.18.79 |
| boot | `boot-imaoff-v4.img` (IMA off) |
| cmdline | `ima_appraise=off ima_tcb module.sig_enforce=0 ...` |
| Пользователь | `user` (uid 100000), `/home/user`, пароль 1234 |
| root | пароль 1234, SSH разрешён (`PermitRootLogin yes`) |
| Доступ | USB RNDIS `192.168.2.15`; Wi-Fi `192.168.1.241` (может меняться) |
| Swap | zram 1 ГБ (pri 100) + `/home/swapfile` 2 ГБ (pri 10) |
| Dev | gcc, make, git, python3 3.8, rpm-build, openssl; **бинарники запускаются** |
| Откат boot | `mtkrun.sh w boot boot-stock-16m.img --skipwdt` (через preloader/DA) |

Дальнейшие шаги (по желанию): поставить Go/Rust (в репо есть `rust`, `cargo`; `go` — проверить имя пакета), Debian chroot для свежих toolchain и утилит (nmap/mc/tmux/vim), настроить `sdk-deploy-rpm`.

---

## 22. Go / Fyne / Wayland на планшете — что реально работает

### Toolchain собран и работает

* **Go 1.22.12 linux-arm** (`/home/opt/go`) — официальный `linux-armv6l` tarball, работает на ARMv7.
  ⚠️ `/home` смонтирован **noexec** — либо `mount -o remount,exec /home`, либо держать Go в `/opt`/`/tmp`.
* **GLFW 3.4** собран из исходников, Wayland-only (X11 отсутствует): `-DGLFW_BUILD_X11=OFF -DGLFW_BUILD_WAYLAND=ON`.
  Понадобилось: `cmake`, `wayland-devel`, `libxkbcommon-devel`, заголовки EGL/GLES2 (взяты распаковкой RPM `mesa-llvmpipe-*-devel` без установки — установка конфликтует с adaptation/libhybris), `linux/input-event-codes.h` (в старых kernel-headers его нет — принесён из upstream), заглушки pkg-config (`libdrm.pc`, `wayland-egl.pc`, `gl.pc`), симлинки `lib*.so`.
* **Fyne v2.4.4** собирается: `go build -tags "wayland gles egl"` (тег `egl` обязателен, иначе go-gl тянет GLX/X11).
  Готовый бинарник: `/home/user/fyne-demo/demo` (23.7 МБ, ARMv7, динамический).

### Но GUI не запускается: у lipstick нет xdg-shell

Список Wayland-глобалов lipstick (`/run/display/wayland-0`), полученный маленьким клиентом на `libwayland-client`:

```
wl_compositor v3, wl_shm v1, wl_seat v3, wl_output v2, wl_data_device_manager v1,
wl_shell v1, qt_surface_extension v1, qt_windowmanager v1, qt_touch_extension v1,
qt_hardware_integration v1, android_wlegl v2, alien_manager v1, lipstick_recorder_manager v1
```

**Нет `xdg_wm_base` и нет `zxdg_shell_v6`** — есть только устаревший `wl_shell` и Qt/Sailfish-специфичные протоколы. GLFW 3.3/3.4 работает через xdg-shell, поэтому:

```
PlatformError: Wayland: Failed to find xdg-shell in your compositor
panic: NotInitialized: The GLFW library is not initialized
```

(дополнительно: Fyne ругается на отсутствие session D-Bus — нужен `DBUS_SESSION_BUS_ADDRESS`).

### Выводы и варианты для GUI

1. **Штатный путь для GUI на Aurora — Qt/QML** (Aurora SDK): приложение запускается через lipstick/booster и поддерживается системой.
2. **Fyne на lipstick — нет**: без xdg-shell окно не создаётся. Патчить GLFW под `wl_shell` (deprecated, нет CSD/декораций) теоретически можно, но это большой форк GLFW.
3. **Fyne с собственным композитором**: остановить `lipstick`, запустить weston/cage/wlroots-композитор с DRM/KMS-бэкендом, в нём — Fyne-приложение. Технически возможно (нужно собрать композитор под kernel 3.18 и Mali/libhybris), но это отдельный проект и «не Aurora UI».
4. **Fyne как «движок» без GUI**: логика/тесты/CLI на Go — работают; интерфейс можно сделать веб-сервером (Go) и открывать в браузере Aurora.

### Итог

* Компилировать Go/C/C++/Rust на планшете — **да** (IMA снят).
* Собирать Fyne-приложения — **да** (toolchain готов).
* Показывать Fyne-окно в Aurora — **нет** (композитор не поддерживает xdg-shell). Для GUI — Qt/QML или собственный композитор.

---

# ОБНОВЛЕНИЕ 2026-11-21: Fyne-окна работают, графика аппаратная

Выводы выше («показывать Fyne-окно в Aurora — нет») **устарели**. Фактическое состояние:

## 1. Fyne-окно в Aurora показывается

Причина прежней неудачи — GLFW искал `xdg-shell`, которого в lipstick нет (есть только
`wl_shell` v1). Решение: небольшой патч GLFW (`wl_shell`-путь + `wl_touch → mouse`),
без форка и без своего композитора. Окно открывается, тач и ввод работают.
Живое приложение: `cterm` (ClearTERM) — собрано и запущено на планшете.

## 2. Графика — аппаратная (Mali-T720), не llvmpipe

* lipstick и Qt-приложения в системе используют `/dev/mali0`, `libGLES_mali.so`,
  `eglplatform_hwcomposer.so` — Aurora целиком на GPU.
* Наш Fyne/GLFW-стек тоже идёт на Mali — достаточно `EGL_PLATFORM=wayland`
  (libhybris сам оборачивает `wl_egl_window` в `WaylandNativeWindow`, буферы — `android_wlegl`).
* Замеры (1200×1920, clear+swap): **Mali ≈13.0 мс/кадр (76.7 fps)** против
  llvmpipe ≈37.3 мс/кадр (26.8 fps) — **≈2.9×**.
* Mesa llvmpipe остаётся резервом; выбор режима — `scripts/run-fyne-gpu.sh`
  (`GPU=auto|hw|sw`) с автоматическим откатом.

## 3. Нужна ли своя прослойка-адаптер

Нет: `wl_surface ↔ EGL` и `android_wlegl` уже реализованы в libhybris. Своё имеет смысл
писать только как:
* тонкий Go-слой `pkg/aurora/gpu` — выбор GPU/CPU, fallback, диагностика;
* позже C-шим для `EGL_KHR_partial_update` (перерисовка только изменённых строк —
  важно для терминала), управления числом буферов и kiosk-режима `EGL_PLATFORM=hwcomposer`.

## 4. Где документация

* `doc/14-HARDWARE-ACCELERATION.md` — аппаратное ускорение: факты, пробник, замеры, адаптер;
* `doc/07-ROADMAP.md` §7.3 — исправленный план (старый опровергнут);
* `patches/glprobe.c` — пробник EGL (Mali vs llvmpipe, замер FPS);
* `scripts/run-fyne-gpu.sh` — запуск с авто-выбором GPU.

> Пути обновлены 2026.11.21 после переноса знаний из `cterm/aurora` в этот репозиторий.

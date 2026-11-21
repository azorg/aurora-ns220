# 08. Факты об устройстве (MVK-2020 / Aurora)

Собрано по результатам работы 19–20.11.2026. Нужно, чтобы не переоткрывать базовые вещи.

## 8.1 Оборудование и ОС

| Параметр | Значение |
|---|---|
| Модель | MVK-2020 (Byterg), variant `aq_ns220`, плата `W1C9` (MT8735A/W) |
| SoC | MediaTek MT6735/MT8737, ARMv7, 4 ядра |
| RAM | 1 960 644 kB (~1.9 ГБ) |
| Диски | eMMC 14.6 ГБ (`mmcblk0`), microSD 29.8 ГБ (`mmcblk1`) |
| Ядро | `3.18.79 #1 SMP PREEMPT Mon Mar 27 14:47:29 UTC 2023 armv7l` |
| ОС | Aurora **4.0.2.340 (Balakovo)** (было 4.0.2.249) |
| Serial | `INS220067941` |
| Экран | 1216×1920 (stride 4864, 32 bpp), ~264 dpi |

## 8.2 Разделы (`/dev/block/platform/mtk-msdc.0/by-name/`)

```
boot p8 (16M)     recovery p9 (16M)   lk p6 (512K)      logo p10 (8M)
items p5 (2M)     system p23 (2.5G)   vendor p22 (512M)  userdata p25 (11G, LVM)
nvdata p19 (32M)  nvram p2 (5M)       metadata p21 (35M) expdb p11 (10M)
tee1 p16 / tee2 p17 (5M)              secro p14 (6M)
protect1 p3 / protect2 p4 (10M)       keystore p15 (8M)  frp p18 (1M)
para p7 (512K)    seccfg p12 (512K)   odmdtbo p20 (16M)  oemkeystore p13 (2M)
flashinfo p26 (16M)                   cache p24 (400M)
```

`userdata` = LVM: `sailfish-root` 2.5 ГБ → `/`, `sailfish-home` 8.5 ГБ → `/home`.

## 8.3 Пути/окружение

* Wayland-сокет lipstick: **`/run/display/wayland-0`** (`WAYLAND_DISPLAY=../../display/wayland-0`
  в окружении lipstick); права `srwxrw-rw- user:privileg`.
* `XDG_RUNTIME_DIR=/run/user/100000` (uid пользователя `user`, он же бывший `defaultuser`).
* Сессионная D-Bus: `unix:path=/run/user/100000/dbus/user_bus_socket`.
* `/home` смонтирован **`noexec`** (`nosuid,nodev,noexec`) → тулчейн/бинарники из `/home`
  не запускаются без `mount -o remount,exec /home`.
* Пользователь: `user` (uid/gid 100000), пароли `user`/`root`/`devel-su` = `1234`.
  SSH: `root@192.168.2.15` (RNDIS) или по Wi-Fi (`192.168.1.x`), `PermitRootLogin yes`.

## 8.4 IMA и подпись кода

* Boot cmdline (заводской):
  ```
  ... selinux=0 ima_appraise=enforce ima_hash=sha256 ima_tcb module.sig_enforce=1 clearmem=on security=diehard ...
  ```
* При `enforce` любой не-ОМР бинарник/скрипт → `EACCES` (execve), `dlopen` своих `.so` →
  `failed to map segment`.
* **Отключение** — правка cmdline **в заголовке boot-образа по смещению `0x40`**
  (поле `0x30..0x3F` — `name[16]`, его нельзя трогать; частая ошибка — писать в `0x38`,
  это ломает `bootopt=` и устройство не грузится):
  ```
  ima_appraise=off  module.sig_enforce=0
  ```
* Рабочий образ: `boot-imaoff-v4.img` (16 МБ, md5 `0ff5bf5bea5e7d8396d1de1cd4467de8`).
* Заводской: `boot-stock-16m.img` (md5 `35c87139ccc8e1a411aeb3ad4380688c`),
  оригинал без паддинга — `boot-stock.img` (md5 `d915a46fd05d2472f1ba1c84e65e86da`).
* Проверка: `cat /proc/cmdline | tr ' ' '\n' | grep -E 'ima_|sig_enforce'`.

## 8.5 Формат boot-образа (для правок)

```
0x0000  ANDROID! header (page_size = 2048)
0x0008  kernel_size (8 066 647)
0x0010  ramdisk_size (4 261 699)
0x0030  name[16]        ← не трогать
0x0040  cmdline[512]    ← сюда писать
0x0240  id[8], 0x0248 extra_cmdline[1024]

0x0800  zImage: magic 0x016f2818 @0x24; stub 0x0000..0x4758
         gzip-payload 0x4758..0x7a1ef8 (7 985 056 байт)
         DTB сразу после payload, totalsize 63 327, конец 0x7b1657 (= kernel_size)
ramdisk: 4 261 699, выровнен на 2048
```

Прошивка: `fastboot flash boot <img>` (fastboot разрешён даже при `unlocked: no`),
либо `dd` в раздел из-под root, либо через preloader/DA (см. §8.6).

## 8.6 Аварийная прошивка через preloader/DA (MTK)

Preloader отдаёт **CDC-ACM** (`/dev/ttyACM*`), протокол MTK:

* handshake `A0 0A 50 05` → `READY` + `5F F5 AF FA`;
* `0xFD` → hw code `0x321` (MT6735); `target config 0x0` → SBC/SLA/DAA **выключены**;
* `0xD8` (GET_TARGET_CONFIG) не поддерживается; `0x70` (SEND_PARTITION_DATA) эхо-подтверждает
  (но обрывать пакет нельзя — preloader зависнет).

`mtkclient` не успевает открыть порт (окно ~2.5 с), поэтому используется **pty-мост**:
`scripts/mtk_bridge7.py` делает handshake и держит сессию, mtkclient работает через
`/tmp/ttyMTK`; `scripts/mtkrun.sh` объединяет шаги.

```bash
# сбросить питание: отключить USB → Power 40–60 с → подключить USB (пойдёт цикл preloader)
scripts/mtkrun.sh w boot boot-stock-16m.img --skipwdt
scripts/mtkrun.sh w boot boot-imaoff-v4.img  --skipwdt
```

Грабли:

* `--skipwdt` обязателен (иначе mtkclient гасит watchdog → вечное зависание);
* запись DA чанками ≤16 КБ (в патче 4 КБ) и ожидание ACK до 5 с;
* `MTK_DA_V5.bin` загружается, stage2 ~0x36620 байт;
* после неудачной сессии preloader часто «висит» → только сброс питания;
* eMMC: 14.6 ГБ, CID `4a54443415010041dcf8c63952068bf7`.

Успешный лог: `Successfully uploaded stage 2` → `Connected to stage2` → `m_emmc_ua_size`
→ `Wrote ... to sector 64512 with sector count 32768`.

## 8.7 Репозитории пакетов

* `ssu lr`: публичные `omp`, `omp-apps`, `hotfixes/aq_ns220`; закрытые
  `omp-hw/adaptation-mediatek-aq_ns220` (401), `pj:hw:common` (401).
* Релизы: 4.0.2.249, 4.0.2.303/310/**340** (последний публичный).
* Файлы: `https://releases.omprussia.ru/releases/<ver>/omp/armv7hl/pj:oss:/armv7hl/<pkg>.rpm`.
* `gpgcheck=0` в zypper-репозиториях.

## 8.7a Графика: GPU и EGL (проверено 2026-11-21)

```
GPU:      Mali-T720 (MT6735), драйвер вендорный /vendor/lib/egl/libGLES_mali.so
Устройство /dev/mali0  (crw-rw-rw- 10,59)
EGL:      libhybris — /usr/lib/libEGL.so.1.0.0 (18 КБ, диспетчер, НЕ Mesa)
Платформы /usr/lib/libhybris/eglplatform_{wayland,hwcomposer,fbdev,null}.so
Mesa:     в системе ОТСУТСТВУЕТ (/usr/lib/dri пуст); llvmpipe ставится в /home/opt/mesa
```

Что загружают процессы: `lipstick` → `/dev/mali0` + `eglplatform_hwcomposer.so`;
Qt-приложения (`booster [browser]`, `booster [silica-qt5]`) → `libEGL` + `libGLES_mali.so`
+ `libwayland-egl`. То есть Aurora целиком на аппаратном GL.

Для наших Fyne-приложений: `EGL_PLATFORM=wayland` (см. `14-HARDWARE-ACCELERATION.md`) →
`GL_RENDERER=Mali-T720`, GLES 3.1. Без неё — ассерт
`ANativeWindow.magic == ANDROID_NATIVE_WINDOW_MAGIC`. Подробности и замеры — в `10`.

## 8.8 Что уже менялось в системе (для «чистого» состояния)

* `boot` → образ с `ima_appraise=off`;
* пользователь `defaultuser` переименован в `user` (uid 100000, `/home/user`), бэкап
  `/root/rename-backup/`;
* swap: `/home/swapfile` 2 ГБ + zram 1 ГБ (units `swapfile.service`, `zram-swap.service`);
* установлено: `gcc`, `make`, `git`, `python3`, `rpm-build`, `openssl`, `htop`, `nano`,
  `screen`, `tcpdump`, `strace`, `cmake`, `wayland-devel`, `libxkbcommon-devel`, `gcc-c++`,
  Qt5 devel (частично);
* `/etc/keys/packages/ca.pem` — дописан наш CA (бэкап `ca.pem.orig-omp`), для обхода
  rpm-валидации; при включённом IMA это не помогает (ключ не в `.ima` keyring);
* `hostname` = `MVK-2020.localdomain` (требование rpmbuild).

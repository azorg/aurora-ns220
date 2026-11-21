# 10. Root, отключение проверки подписи, режимы загрузки, восстановление

Сводка того, что реально проверено на MVK-2020 (Aurora 4.0.2.340). Годится как рецепт для
аналогичных MTK-планшетов.

## 10.1 Root и SSH

* На устройстве есть `devel-su` (setuid root) с паролем, который задан в «Для разработчиков»
  (на нашем экземпляре — `1234`).
* Root-пароль: `passwd root` (был заблокирован: `root:!`).
* SSH: `PermitRootLogin yes` в `/etc/ssh/sshd_config` + `systemctl restart sshd`.
* Доступ по USB (RNDIS): хост получает `192.168.2.14/24`, планшет — `192.168.2.15`,
  `ssh root@192.168.2.15`. Пароль `1234`.
* `echo 1234 | devel-su sh -c '...'` — способ выполнить команду от root без tty.

## 10.2 Зачем отключать проверку подписи (IMA)

Заводской boot cmdline содержит:

```
ima_appraise=enforce ima_hash=sha256 ima_tcb module.sig_enforce=1
```

Это блокирует **любой** не-ОМР код: `execve` своих бинарников/скриптов → `EACCES`,
`dlopen` своих `.so` → `failed to map segment`. Собранный Go/Fyne бинарник не запустится.

## 10.3 Как отключить (проверено)

Параметры лежат в **командной строке заголовка boot-образа**, смещение **0x40**, 512 байт.

⚠️ Поле `0x30..0x3F` — это `name[16]`, его трогать нельзя. Частая ошибка — писать cmdline
по смещению `0x38`: строка сдвигается на 8 байт, теряется `bootopt=`, устройство не грузится.

Было / стало:

```
bootopt=64S3,32N2,32N2 selinux=0 ima_appraise=enforce ima_hash=sha256 ima_tcb module.sig_enforce=1 clearmem=on security=diehard
bootopt=64S3,32N2,32N2 selinux=0 ima_appraise=off     ima_hash=sha256 ima_tcb module.sig_enforce=0 clearmem=on security=diehard
```

Готовые образы (16 МБ, с паддингом до размера раздела):

| Файл | md5 | Что |
|---|---|---|
| `boot-stock-16m.img` | `35c87139ccc8e1a411aeb3ad4380688c` | заводской (IMA enforce) |
| `boot-imaoff-v4.img` | `0ff5bf5bea5e7d8396d1de1cd4467de8` | IMA off |

Прошивка (любой из способов):

```bash
# 1) fastboot (разрешён даже при unlocked: no)
fastboot flash boot boot-imaoff-v4.img

# 2) из-под root в системе
dd if=boot-imaoff-v4.img of=/dev/block/platform/mtk-msdc.0/by-name/boot bs=1M

# 3) через preloader/DA (если система не грузится) — см. §10.5
scripts/mtkrun.sh w boot boot-imaoff-v4.img --skipwdt
```

Проверка: `cat /proc/cmdline | tr ' ' '\n' | grep -E 'ima_|sig_enforce'`.

После этого `gcc hello.c -o hello && ./hello` и собранные Go-бинарники работают.

## 10.4 Режимы загрузки (`boot-mode`)

Скрипт `scripts/boot-mode` (копируется в `/usr/local/bin/`) переключает режим, меняя
образы в разделах. Схема:

```
/boot-modes/boot-imaon.img    IMA enforce (production)
/boot-modes/boot-imaoff.img   IMA off     (dev)
/boot-modes/recovery-stock.img  штатный recovery (для отката)
```

Команды:

```bash
boot-mode status    # что сейчас в boot/recovery + текущий cmdline
boot-mode dev       # boot := IMA off  (обычная загрузка = разработка)
boot-mode secure    # boot := IMA on   (проверка подписи включена)
```

Важно: скрипт работает **даже при включённой проверке**, потому что использует `busybox`
(подписан OMP: у `/usr/bin/busybox` есть `security.ima`). Это гарантирует «выход» из secure-режима.

### Что НЕ сработало (чтобы не повторять)

Идея «положить dev-образ в раздел `recovery` и выбирать режим в boot-меню LK» **не работает**:
пункт «Recovery Mode» в LK грузит **boot-раздел** с флагом `boot-recovery`, а не раздел
`recovery`; наша система в таком режиме висит на логотипе. Раздел `recovery` используется
только штатным recovery-образом (свой формат заголовка).

Следствие: выбор режима делается **до перезагрузки** командой `boot-mode`, либо с хоста
(`fastboot flash boot ...`).

## 10.5 Аварийное восстановление: preloader/DA (MTK)

Preloader отдаёт **CDC-ACM** (`/dev/ttyACM*`), протокол MTK:

* handshake `A0 0A 50 05` → ответ `READY` + `5F F5 AF FA`;
* `0xFD` → hw code `0x321` (MT6735); `target config 0x0` → **SBC/SLA/DAA выключены**;
* `0xD8` (GET_TARGET_CONFIG) не поддерживается; `0x70` (SEND_PARTITION_DATA) эхо подтверждает,
  но обрывать пакет нельзя — preloader зависнет (лечится сбросом питания).

`mtkclient` не успевает открыть порт (окно ~2.5 с), поэтому используется **pty-мост**:
`scripts/mtk_bridge7.py` мгновенно открывает порт, делает handshake и держит сессию;
`scripts/mtkrun.sh` запускает мост, ждёт `BRIDGE HS OK`, затем вызывает mtkclient через
`/tmp/ttyMTK`.

```bash
scripts/mtkrun.sh w boot boot-stock-16m.img --skipwdt
```

Грабли (проверено):

* `--skipwdt` обязателен — иначе mtkclient гасит watchdog и зависание становится вечным;
* DA пишет чанками; в патче mtkclient — 4 КБ и ожидание ACK до 5 с (иначе
  `Data ack failed for sdmmc_write_data`);
* после сессии preloader часто «висит» → только сброс питания (отключить USB, Power 40–60 с);
* успешный лог: `Successfully uploaded stage 2` → `Connected to stage2` → `Wrote ... to sector ...`.

## 10.6 TODO: подпись вместо отключения

Планируется (см. `doc/ai/plans/roadmap.md`):

1. Проверить, принимает ли ядро **hash-аппрайз**: записать в `security.ima` не подпись, а хеш
   (`ima-ng`) и запустить файл — если EVM выключен, это может работать без ключей OMP.
2. Если нет — получать developer-сертификат OMP и подписывать пакеты штатно
   (`rpmsign-external`), тогда `ima_appraise=enforce` можно включать.
3. Проверить `sdk-deploy-rpm` (возможно, он регистрирует сертификат в `.ima` keyring).

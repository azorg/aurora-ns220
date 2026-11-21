# 16. Сводка находок по железу NS220 (состояние на 2026-11-21)

Одна страница «что мы точно знаем про железо и как к нему ходить». Детали —
в `doc/15-PERIPHERALS.md`, баги — в `doc/ai/bugs/README.md`.

## 16.1. Кнопки (evdev, группа `input`, без root)

| Кнопка | Код | Где | Примечание |
|---|---|---|---|
| POWER | 116 | боковая | короткое нажатие — экран; долгое — меню питания (не проверяли) |
| VOLUME UP | 115 | боковая | |
| VOLUME DOWN | 114 | боковая | |
| BACK | 158 | передняя панель | |
| **UP** | **59** | передняя панель | в Linux `KEY_F1`, в прошивке код 59 |
| **DOWN** | **60** | передняя панель | `KEY_F2`, код 60 |
| HOME | 102 | передняя панель | |
| **RESET** | — | утопленная, иголкой | **событий нет**: аппаратный сброс, планшет уходит в ребут |

Чтение: `/dev/input/event1` (mtk-kpd), также event0 (ACCDET) и event2 (elink_keys).
Автопроверка/логгер: `apps/hello-ns220/scripts/btncap.py` (пишет каждое нажатие с
кодом и временем). В приложении коды — `keyNames` в `apps/hello-ns220/periph.go`.

## 16.2. Подсветка

`/sys/class/leds/lcd-backlight/brightness` 0..255 (`max_brightness`=255, обычно 96);
владелец `system:system`, пользователю писать нельзя → setuid-хелпер
`/usr/bin/aurora-brightness` (`aurora-brightness [+N|-N|N]`, печатает «cur max»).
В RPM входит с `%attr(4755,root,root)`.

## 16.3. Сон/экран (mce, системная шина, без root)

`com.nokia.mce` `/com/nokia/mce/request`:
`req_display_state_off` / `req_display_state_on` / `…_dim` / `…_lpm`,
`req_display_blanking_pause` (продление паузы гашения), `get_display_status`.

**Ключевая находка:** при погашенном экране MTK sensors HAL **приостанавливает
сенсоры** — ACC/компас дают 0, подсветка = 0. Поэтому приложение с `-keepawake`
раз в 5 с продлевает `req_display_blanking_pause`. Это же объясняет «залипание»
сенсоров (B045): данные пропадали, когда экран гас по таймауту.

## 16.4. Сенсоры (G-сенсор, компас, ориентация)

Работают через helper `sensor-reader` (QtSensors → sensorfw):
`/usr/libexec/aurora-hello-ns220/sensor-reader` — строки
`ACC x y z ts`, `MAG азимут …`, `ORT код ts`.
Важно: `setlocale(LC_NUMERIC,"C")` — иначе под ru_RU числа идут с запятой (B044).
Прямой Android HAL недоступен (binderized, vtable NULL, B037), D-Bus
`requestSensor` данные не отдаёт и «залипает» (B043).

## 16.5. GPS

`geoclue-provider-hybris` (сессионная шина), объект
`/org/freedesktop/Geoclue/Providers/Hybris`:
`org.freedesktop.Geoclue.GetStatus()` → i (0 ошибка, 1 поиск, 2 данные, 3 фикс),
`org.freedesktop.Geoclue.Position.GetPosition()` → `iiddd(idd)`.
Юнит в прошивке disabled — поднимать `systemctl --user start geoclue-providers-hybris`
(приложение делает это через systemd D-Bus). В помещении фикса нет (норма).

## 16.6. Модем/SIM/звонки

ofono `/ril_0`: online, IMEI `356399112625294`, модем `MOLY.LR9.W1444…`;
`SimManager.GetProperties` → `Present=false` (**SIM нет**);
`NetworkRegistration` в сборке отсутствует, `Emergency=false`.
**112 недоступен** без SIM/регистрации; звонок на 112 мы намеренно не проверяем (B048).

## 16.7. Экран и ориентация

Панель 1200×1920, плотность ~264 dpi. Системный поворот **залочен вендором**
(dconf `screen_rotation_angle=0` + lock, B047): ОС портретная, `orientation_lock`
не действует. Fyne не умеет вращать текст, поэтому в приложении кнопки ⟲/⟳/0°
крутят только сцену.

## 16.8. Звук/вибро

PulseAudio 14.2 (module-droid-card): `sink.primary_output`, `source.droid`.
Звук — `libpulse-simple` через cgo (симлинки `.so` в `/home/opt/libs`, B034);
тон слышен, микрофон в дБ работает. Вибро — `/sys/class/timed_output/vibrator/enable`
(мс, доступно пользователю); клик 60 мс (25 мс не ощущается, B036).

## 16.9. Служебное

* `aurora-sensor-reset` (setuid) — `setprop ctl.restart sensors-hal-1-0` +
  `systemctl restart sensorfwd`; помогает не всегда, надёжнее перезагрузка.
* `debian-enter` (setuid) — Debian 12 chroot без пароля (см. `doc/11`).
* `reboot` иногда приводит к boot-menu LK (`0bb4:0afe`) — нужен выбор Normal (B038).
* `/` (rootfs) всего 2.3 ГБ: сборку RPM держать в `/home/user/rpmbuild` (B046).

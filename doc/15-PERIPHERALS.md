# 15. Периферия из Go: звук, микрофон, вибро, GPS, G-сенсор

Дата: 2026-11-21. Устройство: MVK-2020 (NS220), Aurora 4.0.2.340, ядро 3.18.79.
Код: `apps/hello-ns220/` (`audio.go`, `vibrate.go`, `gps.go`, `accel.go`, `panel.go`).
Всё ниже проверено на живом устройстве; G-сенсор — единственное, что не отдаёт данные
(причина и все попытки — в конце).

## 15.1. Сводка

| Что | Как | Статус |
|---|---|---|
| Звук на динамик | PulseAudio, `libpulse-simple` через cgo (`pa_simple_new/write/drain`) | **работает** (сигнал 440/660 Гц слышен) |
| Микрофон, уровень дБ | тот же `pa_simple_read`, RMS → dBFS | **работает** (тишина ≈ −60 дБ, тон ≈ −38 дБ) |
| Вибромотор | sysfs `/sys/class/timed_output/vibrator/enable`, запись мс | **работает** (доступен пользователю) |
| GPS | geoclue 0.12, провайдер `geoclue-hybris` на **сессионной** шине | **работает** (статус/координаты; фикса в помещении нет) |
| G-сенсор | helper `sensor-reader` через QtSensors → sensorfw (в RPM) | **работает**: ACC/MAG/ORT, но данные пропадают при погашенном экране (см. §15.9) |

## 15.2. Звук (вывод)

Стек: PulseAudio 14.2 (omp) + nemo-модули, `module-droid-card` (MTK).
Устройства (`pactl list short sinks/sources`):

```
0  sink.primary_output   module-droid-card  s16le 2ch 44100Hz
1  sink.deep_buffer     module-droid-card
2  source.droid         module-droid-card   (микрофон)
```

Dev-заголовков и симлинков `.so` на устройстве нет, поэтому:

1. симлинки для линковки: `ln -s /usr/lib/libpulse-simple.so.0 /home/opt/libs/libpulse-simple.so`
   (и так же `libpulse.so`) — `-L/home/opt/libs` уже в `CGO_LDFLAGS`;
2. в `audio.go` прототипы `pa_simple_*` объявлены вручную (`-l:` в `#cgo LDFLAGS`
   Go не принимает, см. bugs B034), `pa_sample_spec` ABI-совместим:
   `{int format; uint32 rate; uint8 channels}`, `PA_SAMPLE_S16LE=3`.

Тон 440 Гц: `pa_simple_new(..., PA_STREAM_PLAYBACK=1, ...)` → `pa_simple_write` →
`pa_simple_drain`. **Проверено на слух** (2026-11-21), а также микрофоном
(уровень при проигрывании поднимается с −60 до −38 дБ).

## 15.3. Микрофон и уровень в дБ

Непрерывный захват: `PA_STREAM_RECORD=2`, моно 44.1 кГц, чанки 1024 сэмпла:

```
rms  = sqrt( Σ s² / N )              (s — int16)
dBFS = 20·log10(rms / 32768)          (предел −90)
```

Сглаживание: быстрая атака, медленный спад (`db = max(db, x)` / `0.75·db + 0.25·x`),
пик держится ~1 с. На устройстве: тишина −60…−66 дБ, голос/тон −40…−32 дБ.
Источник по умолчанию — `source.droid` (не monitor).

## 15.4. Вибромотор

`/sys/class/timed_output/vibrator/enable` (0660 system:input; пользователь `user`
в группе `input`). Запись `<мс>` включает мотор на это время, `0` — выключает.
Дополнительно приложение пишет `0` через `d+100 мс` (страховка). Root не нужен.
Пример: `Vibrate(250*time.Millisecond)` → «enable=250».

Виброклик на тап — **60 мс**, на двойной тап — **90 мс** (25 мс мотор не успевает
раскрутиться и не ощущается, bugs B036); включается/выключается чекбоксом,
в журнал пишется `VIBRO tap 60ms`.

## 15.5. GPS (geoclue 0.12 + geoclue-hybris)

Состав: `/dev/stpgps` (MTK GPS), `mnld`, `vendor.mediatek.hardware.gnss@1.1-service`
— стек Android GNSS живой. Провайдер для приложений — `geoclue-provider-hybris`
(`/usr/libexec/geoclue-hybris`) на **сессионной** шине.

API (проверено вручную `busctl --user` и из Go):

```
сервис  org.freedesktop.Geoclue.Providers.Hybris
объект  /org/freedesktop/Geoclue/Providers/Hybris
  org.freedesktop.Geoclue.GetStatus() -> i           0 ошибка, 1 поиск, 2 данные, 3 фикс
  org.freedesktop.Geoclue.GetProviderInfo() -> (ss)
  org.freedesktop.Geoclue.Position.GetPosition() -> iiddd(idd)
       (status, timestamp, lat, lon, alt, fields)
```

Важные грабли:

* в прошивке юнит `geoclue-providers-hybris.service` **disabled**, а D-Bus-файл
  провайдера указывает `Exec=/bin/false` — одного D-Bus-запуска по имени мало;
  приложение зовёт `org.freedesktop.systemd1.Manager.StartUnit` на **user**-шине
  (идемпотентно; то же делает `systemctl --user start geoclue-providers-hybris`);
* `GetPosition` возвращает структуру `(idd)` — `dbus.Store` в неё не раскладывается
  («length mismatch»), поэтому тело ответа разбирается вручную (bugs B032);
* `org.freedesktop.Geoclue.Master` для прямой работы не нужен: он отвечает
  «no usable Position providers», пока провайдер не получил фикс;
* в помещении статус 1 («поиск»), координаты NaN — это нормально; для фикса нужен
  открытый участок неба.

## 15.6. G-сенсор, компас, ориентация (работает через helper)

Что есть в системе:

```
/dev/gsensor (misc 10:36), /dev/msensor, i2c 2-0018 "gsensor"
sensorfwd.service: com.nokia.SensorService, local.SensorManager
  availableSensorPlugins: accelerometersensor alssensor compasssensor
    magnetometersensor orientationsensor pressuresensor proximitysensor rotationsensor
  loadPlugin("accelerometersensor") -> true
  requestSensor("accelerometersensor", <мс>) -> id
  объект /SensorManager/accelerometersensor, local.AccelerometerSensor
  сигнал dataAvailable(tiii) = (timestamp, x, y, z), scale=1000
```

Данных нет. В журнале sensorfwd при любом запросе:

```
HybrisAdaptor::setInterval(unsigned int, int) setInterval not ok
```

То есть Android-адаптер (`/usr/lib/sensord-qt5/libhybrisaccelerometeradaptor-qt5.so`)
не может задать интервал — HAL (MTK) отвергает `setDelay()`. QtSensors-проверка
подтвердила: сенсор `isActive()=1`, `error()=0`, но `reading` всегда (0,0,0).

Что пробовали (всё безуспешно, детали — bugs B031):

* D-Bus `dataAvailable` — ни одного сигнала (в т.ч. `busctl monitor`);
* `/dev/gsensor` + ioctl `0xF001/0xF002/0xF00A/0xF00B` → `-1` (не тот ABI);
* прямой Android sensors HAL через libhybris (`hw_get_module` → `activate`) —
  список датчиков отдаётся (ACCELEROMETER handle=1), но `activate()` роняет процесс;
* QtSensors (`QAccelerometer`, `-no-...`) — стартует, значения нулевые;
* `/dev/i2c-*` нет (i2c-dev не собран/не создаётся), BMA222-драйвер не привязан
  к i2c-устройству (сенсор за MTK SCP/sensor hub).

Возможные пути дальше: (1) патч/обход `setInterval` для hybris-адаптера
(использовать `batch()` HAL 1.3), (2) свой адаптер через `ISensors` (hwbinder)
по образцу `libhybrissensorfw-qt5`, (3) ядерный путь через SCP, если найдётся
интерфейс. Пока в приложении честно отображается «HAL не отдаёт данные».

## 15.7. Аппаратные кнопки (evdev)

Устройства: `/dev/input/event0` (ACCDET), `event1` (mtk-kpd — клавиши), `event2`
(elink_keys), `event3` (тачскрин). Группа `input` — пользователь `user` в ней есть,
поэтому evdev читается **без root**.

Приложение открывает event0/1/2 и разбирает `struct input_event` (на 32-битном
ARM — 16 байт: `sec,usec uint32; type,code uint16; value int32`), фильтр
`EV_KEY` + `value==1` (нажатие). В панели: «кнопки: VOLUMEUP ×3 (0 с)»,
в журнале — `BUTTON: VOLUMEUP (code 115)`.

### Карта кнопок NS220 (снята вручную 2026-11-21, `scripts/btncap.py`)

| Кнопка | Код evdev | Имя Linux |
|---|---|---|
| **Power** (боковая) | 116 | `KEY_POWER` |
| **Volume Up** (боковая) | 115 | `KEY_VOLUMEUP` |
| **Volume Down** (боковая) | 114 | `KEY_VOLUMEDOWN` |
| **BACK** (передняя панель) | 158 | `KEY_BACK` |
| **UP** (передняя, «вверх») | 59 | `KEY_F1`, в прошивке приходит как код 59 |
| **DOWN** (передняя, «вниз») | 60 | `KEY_F2`, код 60 |
| **HOME** (передняя панель) | 102 | `KEY_HOME` |
| **Reset** (утопленная, иголкой) | — | события нет: аппаратный сброс (планшет уходит в ребут) |

Использование в Go: константы в `apps/hello-ns220/periph.go` (`keyNames`),
чтение `/dev/input/event1` (mtk-kpd). Reset для софта невидим — после него
устройство перезагружается (и может встать в boot-menu LK, см. B038).

## 15.8. Подсветка

`/sys/class/leds/lcd-backlight/brightness` (0..255, сейчас 96; владелец
`system:system`), `max_brightness` = 255. Пользователь писать не может
(`Permission denied`), поэтому в пакете есть setuid-хелпер
`/usr/bin/aurora-brightness` (`packaging/brightness/aurora-brightness.c`):

```sh
aurora-brightness          # "96 255"
aurora-brightness 128      # установить
aurora-brightness +20/-20  # шаг
```

Кнопки «− / +» в панели меняют яркость на 20.

## 15.9. Сон и пробуждение экрана (mce)

`com.nokia.mce` (системная шина), объект `/com/nokia/mce/request`:

```sh
busctl --system call com.nokia.mce /com/nokia/mce/request \
    com.nokia.mce.request req_display_state_off    # погасить
busctl --system call com.nokia.mce /com/nokia/mce/request \
    com.nokia.mce.request req_display_state_on     # включить
# также: req_display_state_dim, req_display_state_lpm,
#        req_display_blanking_pause, get_display_status
```

Доступно пользователю без root. Кнопки «Сон / Экран» в панели.

**Важно (нашлось 2026-11-21):** при **погашенном экране** MTK sensors HAL
приостанавливает сенсоры — ACC/компас отдают нули, подсветка = 0. Поэтому
приложение с флагом `-keepawake` (по умолчанию) каждые 5 с продлевает
`req_display_blanking_pause`, пока открыто. Это же объясняет «залипание» сенсоров
из B045: данные пропадали, когда дисплей гас по таймауту.

## 15.10. Модем, SIM и звонки (в т.ч. 112)

ofono (системная шина), модем `/ril_0`:

```sh
busctl --system call org.ofono / org.ofono.Manager GetModems
# Online=true, Serial=356399112625294 (IMEI), Revision=MOLY.LR9.W1444.MD…
busctl --system call org.ofono /ril_0 org.ofono.SimManager GetProperties
# Present=false  → SIM-карты нет
```

Интерфейсы модема: `VoiceCallManager`, `SimManager`, `OemRaw`, `CellInfo`, `SimInfo`.
`NetworkRegistration` в этой сборке отсутствует (нет SIM) → регистрации в сети нет,
`Emergency=false`.

**112:** позвонить **нельзя** — без SIM нет регистрации в сети, а `VoiceCallManager`
не имеет сети для экстренного вызова. Кроме того, проверять экстренный номер
звонком мы не будем (это реальная служба спасения). Если появится SIM и модем
зарегистрируется, проверять только на тестовом номере оператора; для 112 —
только при реальной необходимости.

## 15.11. Как проверить самому

```bash
cd /home/user/hello-ns220
./run.sh -stats 2s          # на экране: микрофон/GPS; кнопки тона/вибро
# из лога (раз в 2 с):
# PERIPH mic=-58.3dB(peak -53.0) run=true accel=(... ok=false ...) gps=(st=1 ... prov="Android GPS provider")
```

Автопроверка жестов/звука/вибро: `./run.sh -selftest` (в конце — тон 660 Гц и вибро 250 мс).


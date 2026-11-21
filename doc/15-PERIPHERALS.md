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
| G-сенсор | sensorfw (`local.AccelerometerSensor.dataAvailable(tiii)`) | **данных нет**: HAL отвечает `setInterval not ok` (bugs B031) |

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

Виброклик на тап — 25 мс (40 мс на двойной тап), включается/выключается чекбоксом.

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

## 15.6. G-сенсор (акселерометр) — данных нет

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

## 15.7. Как проверить самому

```bash
cd /home/user/hello-ns220
./run.sh -stats 2s          # на экране: микрофон/GPS; кнопки тона/вибро
# из лога (раз в 2 с):
# PERIPH mic=-58.3dB(peak -53.0) run=true accel=(... ok=false ...) gps=(st=1 ... prov="Android GPS provider")
```

Автопроверка жестов/звука/вибро: `./run.sh -selftest` (в конце — тон 660 Гц и вибро 250 мс).

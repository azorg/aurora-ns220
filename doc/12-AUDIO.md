# 12. Звук на Aurora: как пробросить ввод/вывод в Go (через Qt/Aurora API или напрямую)

Задача: дать Fyne/Go-приложению (в перспективе — cterm: звонок/сигнал/запись) **вывод и ввод
звука** на Aurora, не теряя интеграции с системными политиками (громкость, приоритеты,
маршрутизация на BT/наушники).

## 12.1 Что известно про аудио-стек Aurora

* Звук — **PulseAudio** (пакеты `pulseaudio`, `pulseaudio-modules-nemo-*`,
  `pulseaudio-policy-enforcement`, `pulseaudio-module-keepalive`, `pulseaudio-modules-nemo-mainvolume`).
  Это те же модули Nemo/Sailfish: политика (`policy-enforcement`) управляет приоритетами
  потоков по «профилям» — важно для приложений (звонки/медиа/будильники).
* Сокет пользователя: `/run/user/100000/pulse/native` (видели в дереве `/run/user/100000`).
  Значит доступ к серверу можно получить с `XDG_RUNTIME_DIR=/run/user/100000`.
* `audio`-группа: пользователь `user` (uid 100000) в ней состоит.
* Предустановлен `gstreamer1.0` + `nemo-gstreamer1.0-interfaces`; Qt-приложения обычно
  играют/пишут звук через `QAudioSink`/`QAudioSource` (QtMultimedia → PulseAudio).
* Микрофон/динамика работают штатно (в системе есть `ofono`, `pulseaudio` в sessии).

## 12.2 Варианты проброса в Go

### Вариант A (рекомендуемый для начала): libpulse через cgo

Go → cgo → `libpulse-simple.so` (`pa_simple_new/pa_simple_write/pa_simple_read`).

Плюсы: минимальный код, нативный для Aurora стек (политика PulseAudio применяется
автоматически), нет зависимости от Qt.
Минусы: нужны заголовки `pulse/simple.h` (в Aurora нет `pulseaudio-devel` — извлечь из RPM
или собрать из исходников), cgo в проекте.

Схема (эскиз):

```go
/*
#cgo LDFLAGS: -lpulse-simple -lpulse
#include <pulse/simple.h>
#include <pulse/error.h>
*/
import "C"
```

Открывать поток с `PA_STREAM_PLAYBACK` / `PA_STREAM_RECORD`, формат `S16LE`, 48000, 1–2 канала,
`pa_simple_new(NULL, "myapp", PA_STREAM_PLAYBACK, NULL, "playback", &ss, NULL, NULL, &err)`.
Для записи — `PA_STREAM_RECORD`.

### Вариант B: Qt/Aurora API → C ABI → Go

Если нужна именно интеграция с Aurora/Qt (например, `QAudioSink` + политики
`audiosystem-passthrough`, профили, выбор устройства через Qt):
C++-прослойка (`QAudioSink`/`QAudioSource` или `QMediaPlayer`), экспортирующая **C ABI**
(`extern "C"`) и собранная в `.so`; Go вызывает через cgo.

Плюсы: «родной» путь, как у Qt-приложений (политика, профили, микрофон).
Минусы: C++ + cgo + Qt-заголовки (в Aurora QtGui/Quick-девелоперские пакеты урезаны —
`qt5-qtgui-devel` конфликтует с adaptation, заголовки извлекаются из RPM), сложнее сборка.

### Вариант C: отдельный процесс-помощник + IPC

`audio-helper` (C/C++/Qt или Go+libpulse) общается с основным приложением по unix-сокету/
JSON или через `os/exec` stdin-stdout (как `core-daemon` в шаблоне `aurora-hybrid-app`).

Плюсы: не тащим cgo в основной бинарник; можно перезапускать; изоляция падений.
Минусы: задержка IPC (для аудио — нужен буфер), сложнее синхронизация.

**Практическая рекомендация:** начать с **A (libpulse-simple)**, а «родной» Qt-путь (B)
подключить только если упрёмся в политику/профили (звонки, приоритеты, выбор устройства).

## 12.3 Что нужно проверить на устройстве (чек-лист)

1. `ls -l /run/user/100000/pulse/native` — есть ли сокет у пользователя `user`.
2. `PULSE_SERVER=unix:/run/user/100000/pulse/native` — доступен ли сервер от root и от user.
3. Наличие `libpulse.so.0`, `libpulse-simple.so.0`:
   `ls /usr/lib/libpulse*.so*`.
4. Заголовки: в репо поискать `pulseaudio-devel`, иначе распаковать из RPM/собрать.
5. Тест вывода: короткий тон 440 Гц 1 с через `pa_simple_write` (или `aplay`, если есть).
6. Тест ввода: 1 с записи с микрофона, убедиться, что данные не нулевые.
7. Проверить политику: не блокирует ли `policy-enforcement` поток без профиля
   (возможно, нужно `pa_context_set_role`/`media.role`, как делают Nemo-приложения).
8. Проверить одновременную работу с системным звуком (микс), громкость, переключение на BT.

## 12.4 Риски и ограничения

* `module.sig_enforce=0`/`ima_appraise=off` нужны только для **наших** бинарников; системный
  PulseAudio — подписан, работает всегда.
* Если приложение запускается от `root` (мы так делали), PulseAudio может отказать:
  сервер принадлежит пользователю `user`, нужно `XDG_RUNTIME_DIR=/run/user/100000` и/или
  запуск от `user`.
* Для записи звука может требоваться разрешение в политике безопасности Aurora
  (`sailjail`-профили для «настоящих» приложений). Для самодельного бинарника вне sailjail
  ограничений обычно меньше, но проверить.
* Задержка через llvmpipe-путь GUI и аудио-буферы не связаны (разные подсистемы).

## 12.5 Связь с cterm

Для терминала звук нужен для: bell (BELL-символ, `\a`), опционально — звуковые уведомления
и «звонок» в SSH-сессиях. Минимально достаточно **вывода** коротких тонов (libpulse-simple,
PA_STREAM_PLAYBACK). Ввод (микрофон) для cterm не нужен, но пригодится для будущих
«терминал + голос» сценариев.

См. также `doc/07-ROADMAP.md` и `doc/ai/plans/roadmap.md`.

# 07. Дорожная карта: tray, ввод, GPU, автоматизация

## 7.1 trayIcon (иконка в области уведомлений)

В Aurora **нет системного трея**: `lipstick` предоставляет только `wl_shell`/Qt-протоколы,
StatusNotifier (KDE/AppIndicator) отсутствует.

### Вариант A (рекомендуемый): свой «трей-хост» на Qt/QML

1. Пишем маленькое Qt/QML-приложение **`aurora-tray`** (в Aurora Qt работает штатно через
   `wl_shell`): горизонтальная панель-полоска с иконками.
2. Fyne-приложения при старте публикуют пункт в D-Bus:
   * сервис `ru.aurora.FyneTray`;
   * метод `Register(name, icon, menu)` → возвращает `objectPath`;
   * сигналы `Activated`, `MenuSelected`;
   * меню — массив `{id, label, enabled}`.
3. `aurora-tray` на `Register` создаёт иконку (из файла/SVG/`image/png` через fd),
   по тапу шлёт `Activated` или `MenuSelected(id)`.
4. Панель можно показывать как небольшое окно (top-center), сворачиваемое в «точку» —
   это компенсирует отсутствие системной панели.

Плюсы: полностью в нашей власти, Qt-путь проверен, D-Bus в системе есть.
Минусы: панель — отдельный процесс; нет интеграции с системным свайпом (можно
эмулировать «жестом» — окно у края экрана).

### Вариант B: уведомления `ngfd`/notification

Иконка как «живое уведомление» — работает, но UX плохой (уведомления уходят).

### Вариант C: иконка внутри самого Fyne-приложения

Fyne умеет рисовать «свёрнутое» состояние в маленьком окне (`wl_shell` без декораций);
при тапе — разворачивать основное окно. Просто, но не «системно».

### Что нужно от Fyne

Fyne API трея (`desktop.App.SetSystemTrayIcon`) в Linux использует `fyne.io/systray`
(StatusNotifier). Для Aurora: подменить реализацию на наш D-Bus-мост (патч `systray`
или свой пакет `fyne-aurora-tray` с тем же интерфейсом).

## 7.2 Тач-ввод

* `wl_seat` v3 в Aurora отдаёт `wl_touch` (проверить: `WAYLAND_DEBUG=1` при касании —
  искать `wl_touch`).
* GLFW умеет `wl_touch` → Fyne получает touch как «мышь + жесты» (тап = клик, drag = скролл).
* Задачи:
  1. проверить события касания (`wl_touch.down/up/motion`);
  2. проверить долгие тапы/инерционный скролл в Fyne (у Fyne есть `Scroll` и `Tappable`);
  3. при необходимости — маппинг в `qt_touch_extension` (композитор его объявляет),
     но обычно достаточно `wl_touch`.
* Виртуальная клавиатура: в Aurora её предоставляет `maliit-server` (для Qt-приложений).
  Для Fyne нужен либо свой ввод (Fyne рисует поля и может вызвать системную клавиатуру
  через `text-input` протокол — проверить наличие `zwp_text_input_manager`), либо
  собственная экранная клавиатура на canvas.

## 7.3 Аппаратный GL — СДЕЛАНО (см. `14-HARDWARE-ACCELERATION.md`)

Прежний план («написать клиент `android_wlegl` и Android-путь в GLFW») **опровергнут**:
мост уже реализован в libhybris (`eglplatform_wayland.so`, `WaylandNativeWindow`).

Достаточно:
```bash
unset LD_LIBRARY_PATH LIBGL_DRIVERS_PATH MESA_LOADER_DRIVER_OVERRIDE GALLIUM_DRIVER
export EGL_PLATFORM=wayland
```
Результат: `GL_RENDERER=Mali-T720`, GLES 3.1; на 1200×1920 ≈13.0 мс/кадр против
37.3 мс/кадр у llvmpipe (**≈2.9×**).

Осталось: тонкий Go-слой выбора режима (`CTERM_GPU=auto|hw|sw`, уже есть в
`scripts/run-cterm-hw.sh` как shell-лаунчер) + позже C-шим для
`EGL_KHR_partial_update` / числа буферов / kiosk-режима `hwcomposer`.

## 7.4 Автоматизация и упаковка

1. Собрать `aurora-fyne-runtime` RPM (см. `06-PACKAGING.md`) и положить его в наше
   локальное хранилище/репозиторий.
2. Добавить `scripts/setup-toolchain.sh` (идемпотентный) — уже есть в `scripts/`.
3. CI-подобный сценарий: сборка демо + smoke-тест (`WAYLAND_DEBUG` → есть `attach`).
4. Шаблон проекта: `fyne new`-аналог (cookiecutter/скрипт), который сразу ставит
   `replace` на патченный GLFW и примешивает `run-fyne.sh`.

## 7.5 Тестовая матрица (что проверять при каждом изменении)

| Проверка | Как |
|---|---|
| патч GLFW применён | `grep -c wl_shell .../wl_window.c` |
| окно создаётся | `WAYLAND_DEBUG=1` → ровно 1 `get_shell_surface` |
| окно рисуется | `attach` ≥ 1, `damage` ≥ 1 |
| нет ANR | нет диалога «приложение не отвечает» |
| масштаб | текст читаем при `FYNE_SCALE=2.5` |
| нет падений | `dmesg`/stderr без SIGABRT |
| CPU | llvmpipe всегда >0, но не 100% в простое |

## 7.6 Возможные улучшения в будущем

* **`wl_shell` → собственный композитор-прокси**: клиент xdg-shell снаружи, wl_shell внутрь.
  Тогда заработают любые xdg-shell тулкиты (GTK4, Qt6, SDL3), а не только GLFW.
* **Скриншоты**: клиент `lipstick_recorder_manager` (запись экрана/скриншот в файл) —
  полезно для автотестов и документации.
* **`sdk-deploy-rpm`**: штатный путь доставки пакетов из Aurora SDK (проверить, регистрирует
  ли он developer-сертификат в `.ima` keyring — тогда подпись станет «правильной»).
* **RnD-репозитории** (`rnd-dist-upgrade`, домен `community`) — возможно, есть dev-сборки
  с xdg-shell.

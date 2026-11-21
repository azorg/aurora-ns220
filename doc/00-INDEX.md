# 00. Индекс документации

Единая база знаний по планшету **MVK-2020 (NS220, Byterg)** под **Aurora OS 4.0.2.340**
(MT6735/MT6737t, ARMv7, Mali-T720, ядро 3.18.79) и по разработке на нём
приложений **Go + Fyne**.

Документы 09–14 и `FYNE-ON-AURORA.md` перенесены из `~/work/clear-ssh/cterm/aurora`
(2026.11.21) и являются здесь каноническими; `REPORT-*.md` в корне — хронология
сеансов работы с устройством.

## Структура репозитория

```
aurora-ns220/
├── AGENTS.md              ← правила работы AI-агента
├── TODO.txt               ← актуальный план (одна задача = одна строка)
├── ChangeLog.ai           ← журнал действий агента
├── REPORT-*.md            ← отчёты по сеансам работы с устройством
├── doc/                   ← документация (этот индекс)
│   └── ai/                ← plans / bugs / audit / notes (см. doc/ai/README.md)
├── apps/                  ← тестовые приложения (hello-ns220 и далее)
├── patches/               ← патчи GLFW, утилиты Wayland (wlinfo, wltest, glprobe)
├── scripts/               ← boot-mode, debian-chroot, сборка/запуск, скриншот
├── tests/                 ← тесты-пробы (жесты, клавиатура)
├── template/              ← шаблон приложения Aurora (core + ui-fyne + ui-qt)
├── osk/                   ← исследование экранных клавиатур (PNG — во внешнем хранилище)
├── bin/, lk/              ← бинарные артефакты IMA/rpm-validation и LK
└── mtk_*.sh/.py, part_write.py, runroot, root_cmd.sh, dumpall.sh  ← работа с устройством
```

## Документы

| # | Файл | О чём |
|---|---|---|
| — | [FYNE-ON-AURORA.md](FYNE-ON-AURORA.md) | **мастер-документ**: Fyne на Aurora, всё в одном |
| 00 | 00-INDEX.md | этот индекс |
| 01 | [01-ARCHITECTURE.md](01-ARCHITECTURE.md) | архитектура решения, отвергнутые варианты |
| 02 | [02-GLFW-PATCH.md](02-GLFW-PATCH.md) | патч GLFW (`wl_shell`, `wl_touch`) + ловушка cgo-кэша |
| 03 | [03-TOOLCHAIN.md](03-TOOLCHAIN.md) | установка тулчейна (Go, заголовки, pkg-config, Mesa) |
| 04 | [04-BUILD-RUN.md](04-BUILD-RUN.md) | сборка/запуск, теги, переменные окружения |
| 05 | [05-TROUBLESHOOTING.md](05-TROUBLESHOOTING.md) | симптомы и диагностика |
| 06 | [06-PACKAGING.md](06-PACKAGING.md) | RPM `aurora-fyne-runtime`, установка |
| 07 | [07-ROADMAP.md](07-ROADMAP.md) | trayIcon, GPU, автоматизация |
| 08 | [08-DEVICE-FACTS.md](08-DEVICE-FACTS.md) | факты об устройстве: разделы, cmdline, репозитории, GPU |
| 09 | [09-TOUCH-AND-KEYBOARD.md](09-TOUCH-AND-KEYBOARD.md) | тач/жесты/клавиатура: замеры |
| 10 | [10-ROOT-AND-IMA.md](10-ROOT-AND-IMA.md) | root, отключение проверки подписи, boot-mode, восстановление |
| 11 | [11-DEBIAN-CHROOT.md](11-DEBIAN-CHROOT.md) | Debian chroot: apt, утилиты |
| 12 | [12-AUDIO.md](12-AUDIO.md) | звук: PulseAudio/Qt → C/C++ → Go |
| 13 | [13-GLFW-V34-AND-WAYLAND119.md](13-GLFW-V34-AND-WAYLAND119.md) | GLFW v3.4 + libwayland 1.19: патч, протоколы, сборка |
| 14 | [14-HARDWARE-ACCELERATION.md](14-HARDWARE-ACCELERATION.md) | аппаратное ускорение: Mali-T720, libhybris, замеры, адаптер |
| 15 | [15-PERIPHERALS.md](15-PERIPHERALS.md) | периферия из Go: звук, микрофон дБ, вибро, GPS, G-сенсор, кнопки, подсветка, сон, модем |
| 16 | [16-FINDINGS-NS220.md](16-FINDINGS-NS220.md) | **сводка находок по железу**: кнопки (POWER/vol/BACK/UP=59/DOWN=60/HOME, Reset), подсветка, сон, сенсоры, GPS, SIM |

## Материалы (doc/ai/)

| Файл | Содержание |
|---|---|
| [ai/README.md](ai/README.md) | соглашения: форматы планов, багов, аудитов, журналов |
| [ai/bugs/README.md](ai/bugs/README.md) | реестр проблем B001–B022: симптом → причина → решение |
| [ai/bugs/osk-host-bugs.md](ai/bugs/osk-host-bugs.md) | отдельный разбор проблем OSK |
| [ai/audit/0001.audit-checklist-new-device.md](ai/audit/0001.audit-checklist-new-device.md) | чек-лист аудита нового устройства |
| [ai/plans/](ai/plans/) | планы 0002–0009 и 0015 (обновление ОС, IMA-режим, chroot, cterm, OSK, RPM, подпись, демо-приложение) |
| [ai/notes/experiment-log.md](ai/notes/experiment-log.md) | журнал экспериментов (19–21.11.2026) |

## Приложения

| Каталог | Что это |
|---|---|
| [apps/hello-ns220/](../apps/hello-ns220/) | «Hello world»: Go+Fyne+OpenGL, тач-точка, жесты, окружение на экране, RPM+подпись (план 0015) |
| [tests/gesture-keyboard/](../tests/gesture-keyboard/) | ранняя проба: жесты/скролл/ввод |
| [template/aurora-hybrid-app/](../template/aurora-hybrid-app/) | шаблон: Go-core + Fyne UI + Qt UI |

## Быстрый старт (на планшете)

```bash
# переменные стенда
. /home/user/clear-ssh/cterm/aurora/env.sh
# сборка приложения (пример — наше демо)
cd /home/user/hello-ns220 && ./build.sh && ./run.sh
# режим проверки подписи
scripts/boot-mode status
# Debian-окружение
scripts/debian-chroot
```

## Главные правила (коротко)

1. **IMA**: свои бинарники работают при `ima_appraise=off` (см. 10, `scripts/boot-mode dev`);
   штатный путь без отключения — подпись своими ключами (TODO, план 0009).
2. **`/home` может быть noexec** → `mount -o remount,exec /home` (скрипты делают это сами).
3. **Патч GLFW** обязателен: в Aurora нет `xdg-shell` (только `wl_shell`).
4. **После правки `.c` в GLFW** — `go clean -cache`.
5. **EGL**: системный libhybris/Mali работает при `EGL_PLATFORM=wayland` (аппаратно, см. 14);
   Mesa llvmpipe через `LD_LIBRARY_PATH` — резерв.
6. **Тач** приходит как `wl_touch` — нужен патч (`wl_touch → mouse`).
7. **Клавиатура** системная недоступна сторонним клиентам → своя экранная (EN/RU).
8. **Восстановление**: `scripts/mtkrun.sh w boot boot-stock-16m.img --skipwdt` (preloader/DA).
9. **Крупные файлы** (> ~1 МБ) — в `~/aurora-ns220/`, не в git (см. `AGENTS.md`).

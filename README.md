# aurora-ns220 — база знаний: планшет NS220 (MVK-2020) под Aurora OS

Всё о планшете **MVK-2020 (NS220, Byterg)**: прошивка, root, IMA (проверка подписи),
восстановление после «кирпича», и о разработке на нём приложений **Go + Fyne + OpenGL**.

Целевая задача проекта: превратить планшет в полноценный стенд разработки —
Debian chroot, RPM-пакеты, свои приложения в меню Aurora, доступ ко всей периферии
из Go, своя RU/EN экранная клавиатура. План — [`TODO.txt`](TODO.txt).

## Быстрая навигация

| Файл | Что внутри |
|---|---|
| [`TODO.txt`](TODO.txt) | актуальный план: что сделано, что в работе, что дальше |
| [`AGENTS.md`](AGENTS.md) | правила работы (для AI-агента и человека) |
| [`ChangeLog.ai`](ChangeLog.ai) | журнал действий агента |
| [`REPORT-aurora-MVK-2020-2026-11-19.md`](REPORT-aurora-MVK-2020-2026-11-19.md) | большой отчёт: обновление ОС, root, IMA, brick и восстановление, Fyne |
| [`LINUX-ON-NS220-NOTES.txt`](LINUX-ON-NS220-NOTES.txt) | железо и периферия, план установки обычного Linux |
| [`doc/00-INDEX.md`](doc/00-INDEX.md) | **индекс документации** (документы 01–14, мастер-документ Fyne) |
| [`doc/ai/`](doc/ai/README.md) | планы, реестр багов, аудиты, журнал экспериментов |
| [`apps/hello-ns220/`](apps/hello-ns220/) | тестовое приложение: Fyne+OpenGL, тач-точка, жесты, RPM |

## Ключевые факты

| | |
|---|---|
| Устройство | MVK-2020 (`aq_ns220`), MT6735A/W, 4×A53 ARMv7, ~1.9 ГБ RAM, Mali-T720 |
| ОС | Aurora 4.0.2.340 (Balakovo), ядро 3.18.79, LVM `sailfish-root`/`sailfish-home` |
| Доступ | SSH `user@192.168.2.15` (USB RNDIS), пароль `1234`; root — `devel-su` (тоже `1234`) |
| IMA | выключен своим boot-образом `boot-imaoff-v4.img`; переключение — `scripts/boot-mode` |
| Графика | Fyne/GLFW работают в lipstick через `wl_shell` (патч) и Mali (libhybris, `EGL_PLATFORM=wayland`) |
| Восстановление | MTK preloader/DA: `mtkrun.sh w <part> <image> --skipwdt` (мост `mtk_bridge7.py`) |
| Крупные файлы | вне git: `~/aurora-ns220/{bootdump,fw,meta,parts}`, `~/aurora-ns220/build`, `~/aurora-ns220/osk-screens` |

## Что уже работает (кратко)

* Root + SSH на устройстве, обновление Aurora 4.0.2.249 → 4.0.2.340 (793 пакета).
* Отключение IMA appraise (`boot-imaoff-v4.img`), свои бинарники/скрипты/`.so` исполняются;
  переключение dev/secure — `scripts/boot-mode`, восстановление — preloader/DA.
* Go 1.22/1.27 на устройстве, GLFW 3.4 (Wayland-only, патченный), Fyne 2.4/2.8:
  окно рисуется, тач работает, GPU — Mali-T720 (~77 fps против ~27 у llvmpipe).
* `cterm` (ClearTERM) — Fyne-терминал с экранной клавиатурой (`pkg/fyneosk`), живёт в
  `~/work/clear-ssh/cterm`.
* Тестовое приложение `apps/hello-ns220` — «Hello world» с анимацией на OpenGL
  (Mali ≈ 40–46 fps, llvmpipe ≈ 16–18 при 1200×1920), точкой касания, распознаванием
  жестов и сведениями об окружении; собрано на устройстве, упаковано и подписано:
  `aurora-hello-ns220-arm7-signed.rpm` (armv7hl) — ставится и запускается.

## Чего ещё нет (см. TODO)

Debian chroot не развёрнут; RPM для устройства не собран; подписи своими ключами нет
(IMA выключен как dev-режим); запуск из меню Aurora не оформлен; периферия из Go
(аудио, сенсоры, GPS, GSM, кнопки, сон, SD) — в планах; универсальный стенд для
других архитектур/версий Aurora — в планах.

## Правило про большие файлы

Репозиторий хранится в git как история проекта, поэтому образы, прошивки, дампы,
rootfs, RPM и серии скриншотов лежат **вне** него — в `~/aurora-ns220/`.
Подробности и порог — [`AGENTS.md`](AGENTS.md), раздел «Крупные файлы».

# RPM-спецификация демо-приложения aurora-hello-ns220 (планшет NS220 / Aurora).
#
# Сборка (на планшете, где есть rpm-build):
#   packaging/build-rpm.sh            # соберёт бинарник (если нет) и пакет
#
# Бинарник ожидается в ./hello-ns220 (см. build.sh). Пакет ставится в
# /usr/bin + .desktop + иконка; запуск — из меню Aurora (липстик) или из shell.
#
# Подпись: пакет можно подписать своим GPG-ключом (`rpm --addsign`), но
# валидатор Aurora (validation.so) доверяет только developer-сертификату OMP —
# см. doc/ai/plans/0009.plan-signing.md.

Name:           aurora-hello-ns220
Version:        1.0.0
Release:        14%{?dist}
Summary:        Go+Fyne+OpenGL demo for Aurora OS on NS220 (MVK-2020)
License:        MIT
URL:            https://example.local/aurora-ns220
BuildArch:      armv7hl
Source0:        hello-ns220
Source1:        aurora-hello-ns220.desktop
Source2:        aurora-hello-ns220.png
Source3:        aurora-hello-ns220-launch.sh
Source4:        aurora-hello-ns220-86.png
Source5:        sensor-reader
Source6:        aurora-brightness.c
Source7:        sensor-reset.c

# Бинарник — динамически линкованный ELF (glibc, wayland, libhybris EGL).
# Зависимости с soname rpm проставит сам (find-requires); явно — только то,
# что не выражается в soname, но нужно на старте.
Requires:       /usr/bin/env
Requires:       pulseaudio
Requires:       qt5-qtsensors
Requires:       qt5-qtsensors-plugin-sensorfw

%description
Демонстрационное приложение «Hello world» для планшета NS220 (MVK-2020) под
Aurora OS: Go + Fyne + OpenGL (Mali-T720 через libhybris), анимация с FPS,
индикация точки касания, распознавание тапа/двойного тапа/долгого нажатия/
свайпов/перетаскивания/прокрутки, на экране — сведения об окружении и режиме
проверки подписи (IMA).

%prep

%build
gcc -O2 -o aurora-brightness %{SOURCE6}
gcc -O2 -o aurora-sensor-reset %{SOURCE7}

%install
install -d %{buildroot}%{_bindir}
install -m 0755 %{SOURCE0} %{buildroot}%{_bindir}/aurora-hello-ns220
install -m 0755 %{SOURCE3} %{buildroot}%{_bindir}/aurora-hello-ns220-launch
install -d %{buildroot}%{_libexecdir}/aurora-hello-ns220
install -m 0755 %{SOURCE5} %{buildroot}%{_libexecdir}/aurora-hello-ns220/sensor-reader
install -m 0755 aurora-brightness %{buildroot}%{_bindir}/aurora-brightness
install -m 0755 aurora-sensor-reset %{buildroot}%{_bindir}/aurora-sensor-reset

install -d %{buildroot}%{_datadir}/applications
install -m 0644 %{SOURCE1} %{buildroot}%{_datadir}/applications/aurora-hello-ns220.desktop

install -d %{buildroot}%{_datadir}/icons/hicolor/256x256/apps
install -m 0644 %{SOURCE2} %{buildroot}%{_datadir}/icons/hicolor/256x256/apps/aurora-hello-ns220.png

# Aurora/lipstick берёт иконку из темы: image://theme/<Icon> ->
# /usr/share/themes/aurora-default/meegotouch/z1.0/icons/<Icon>.png
# (в /usr/share/icons/hicolor нет index.theme, Qt оттуда иконку не находит).
install -d %{buildroot}%{_datadir}/themes/aurora-default/meegotouch/z1.0/icons
install -m 0644 %{SOURCE4} %{buildroot}%{_datadir}/themes/aurora-default/meegotouch/z1.0/icons/aurora-hello-ns220.png

%post
# Подсказка при установке: без EGL_PLATFORM=wayland системный libhybris-EGL
# падает на ANativeWindow-ассерте; .desktop уже задаёт переменную сам.
echo "aurora-hello-ns220: запуск из меню Aurora или:"
echo "  /usr/bin/aurora-hello-ns220-launch"
echo "  режим подписи: $(sed -n 's/.*ima_appraise=\([a-z]*\).*/\1/p' /proc/cmdline) (см. scripts/boot-mode)"

%postun
# Обновить кэш иконок/меню, если инструменты есть (в Aurora — не обязательно).
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database %{_datadir}/applications >/dev/null 2>&1 || :

%files
%{_bindir}/aurora-hello-ns220
%{_bindir}/aurora-hello-ns220-launch
%{_libexecdir}/aurora-hello-ns220/sensor-reader
%attr(4755,root,root) %{_bindir}/aurora-brightness
%attr(4755,root,root) %{_bindir}/aurora-sensor-reset
%{_datadir}/applications/aurora-hello-ns220.desktop
%{_datadir}/icons/hicolor/256x256/apps/aurora-hello-ns220.png
%{_datadir}/themes/aurora-default/meegotouch/z1.0/icons/aurora-hello-ns220.png

%changelog
* Sat Nov 21 2026 AI <ai@local> - 1.0.0-14
- Крупная тема панели: текст/кнопки/чекбоксы ~1.6x по тексту и 2x по отступам
  (bigTheme), полоса микрофона 620x34, подписи панели 22pt. Карта кнопок:
  POWER=116, VOLUMEUP=115, VOLUMEDOWN=114, BACK=158, UP=59, DOWN=60, HOME=102.

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-13
- Кнопки/подсветка/сон/SIM: evdev-кнопки, яркость через setuid-хелпер, сон через
  mce, модем/SIM/IMEI через ofono. -keepawake: приложение продлевает паузу
  гашения экрана (при погашенном экране MTK HAL приостанавливает сенсоры, B045).

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-12
- Кнопки (evdev), подсветка (setuid aurora-brightness 4755), сон/экран (mce),
  модем/SIM/IMEI (ofono). 112: без SIM и регистрации недоступно (B048).
- В пакет добавлены setuid-хелперы aurora-brightness и aurora-sensor-reset.

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-11
- Ручной поворот сцены кнопками ⟲ 90° / ⟳ 90° / 0° (системный поворот в Aurora 4
  на NS220 залочен вендором, B047); флаг -rot N для запуска с поворотом;
  касания пересчитываются в координаты сцены.

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-10
- FIX G-сенсора: helper печатал числа с запятой (локаль ru_RU) — «ACC 0,873 …»,
  а Go-парсер ждал точку; теперь setlocale(LC_NUMERIC,"C") в helper и
  нормализация запятой в парсере. На экране: x/y/z, |g|, тангаж/крен, компас,
  положение (портрет/ландшафт/экраном вверх).

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-9
- G-сенсор: если helper не даёт ACC за 5 с — перезапуск; после трёх неудач —
  сброс sensorfwd через setuid `aurora-sensor-reset` (B043: D-Bus requestSensor
  без releaseSensor оставляет сенсор активным).

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-8
- G-сенсор/компас/ориентация: helper sensor-reader (QtSensors → sensorfw) в
  %{_libexecdir}/aurora-hello-ns220/sensor-reader; приложение запускает его и
  показывает ускорение, |g|, тангаж/крен, азимут и положение (QOrientationReading).
  D-Bus requestSensor больше не используется (оставлял сенсор активным, B043).
- Requires: qt5-qtsensors, qt5-qtsensors-plugin-sensorfw.

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-7
- Кнопка «Выход» в панели (a.Quit), журнал жизненного цикла (WINDOW closed /
  APP stopped / SIGNAL), корректный выход по SIGTERM/SIGINT.
- Один экземпляр: сокет в $XDG_RUNTIME_DIR — повторный запуск просит первый
  показать окно и выходит (раньше оставался невидимый процесс при закрытии
  из Aurora, B040).

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-6
- Виброклик: 25 мс мотор не успевал раскрутиться -> 60 мс (тап) и 90 мс
  (двойной), запись в журнал VIBRO (B036).
- Видимая реакция на прокрутку: полоса прокрутки справа, счётчик
  «прокрутка всего» в HUD и всплывающее «⇅ …» у точки касания.

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-5
- Запуск из меню: Exec — сам ELF-бинарник (не shell-скрипт), окружение
  (XDG/Wayland/EGL/локаль/Compose) выставляет setupEnv(), журнал — setupLog()
  в ~/hello-ns220.log. В .desktop: X-Nemo-Application-Type=no-invoker,
  TryExec, StartupNotify, StartupWMClass (B035).

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-4
- Периферия в одном окне: тон (libpulse-simple), микрофон в дБ, виброклик,
  GPS (geoclue-hybris), G-сенсор (sensorfw; на этой прошивке HAL не отдаёт данные).
- Двухпальцевая прокрутка (патч GLFW v2): 2 пальца -> scroll, 1 палец -> мышь.
- Зависимость: pulseaudio (libpulse-simple.so.0) — добавлена явно.

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-3
- Иконка в тему Aurora (/usr/share/themes/aurora-default/.../icons) — иначе
  lipstick не находит image://theme/<Icon> (B030); добавлен размер 86x86.

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-2
- .desktop: отключена песочница sailjail (Sandboxing=Disabled) — без этого
  приложение из меню падало после запроса разрешений (bugs B029); убран
  нестандартный X-Nemo-Application-Type=no-invoker.
- Лаунчер пишет журнал в $HOME/hello-ns220.log (из меню нет терминала).

* Sat Nov 21 2026 AI <ai@local> - 1.0.0-1
- Первая сборка: Fyne+OpenGL демо с тач-вводом и HUD для NS220/Aurora.

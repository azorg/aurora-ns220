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
Release:        6%{?dist}
Summary:        Go+Fyne+OpenGL demo for Aurora OS on NS220 (MVK-2020)
License:        MIT
URL:            https://example.local/aurora-ns220
BuildArch:      armv7hl
Source0:        hello-ns220
Source1:        aurora-hello-ns220.desktop
Source2:        aurora-hello-ns220.png
Source3:        aurora-hello-ns220-launch.sh
Source4:        aurora-hello-ns220-86.png

# Бинарник — динамически линкованный ELF (glibc, wayland, libhybris EGL).
# Зависимости с soname rpm проставит сам (find-requires); явно — только то,
# что не выражается в soname, но нужно на старте.
Requires:       /usr/bin/env
Requires:       pulseaudio

%description
Демонстрационное приложение «Hello world» для планшета NS220 (MVK-2020) под
Aurora OS: Go + Fyne + OpenGL (Mali-T720 через libhybris), анимация с FPS,
индикация точки касания, распознавание тапа/двойного тапа/долгого нажатия/
свайпов/перетаскивания/прокрутки, на экране — сведения об окружении и режиме
проверки подписи (IMA).

%prep

%build

%install
install -d %{buildroot}%{_bindir}
install -m 0755 %{SOURCE0} %{buildroot}%{_bindir}/aurora-hello-ns220
install -m 0755 %{SOURCE3} %{buildroot}%{_bindir}/aurora-hello-ns220-launch

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
%{_datadir}/applications/aurora-hello-ns220.desktop
%{_datadir}/icons/hicolor/256x256/apps/aurora-hello-ns220.png
%{_datadir}/themes/aurora-default/meegotouch/z1.0/icons/aurora-hello-ns220.png

%changelog
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

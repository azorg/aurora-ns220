# 03. Установка окружения на планшете

Всё ставится в `/home/opt` (чтобы не занимать `/`, где ~250 МБ свободно).
Автоматизация: [`../scripts/setup-toolchain.sh`](../scripts/setup-toolchain.sh).

## 3.0 Предусловия

1. **IMA отключён** (`ima_appraise=off`, `module.sig_enforce=0`) — иначе ни Go, ни собранные
   бинарники не запустятся. См. `08-DEVICE-FACTS.md` §4.
2. `/home` смонтирован с `noexec` (по умолчанию в Aurora) → Go не стартует:

```bash
mount -o remount,exec /home        # или держать тулчейн в /opt, /tmp
```

3. Свободное место: Go ~400 МБ + Mesa ~22 МБ + исходники ~100 МБ.

## 3.1 Пакеты из репозиториев OMP

```bash
zypper -n install cmake wayland-devel libxkbcommon-devel gcc-c++ \
                 qt5-qtdeclarative-devel qt5-qmake      # Qt — по желанию (для QML)
```

Что даёт: `cmake`, `wayland-scanner`, `wayland-client.pc`, `xkbcommon.pc`, `g++`.

Репозитории публичные: `omp`, `omp-apps`, `hotfixes` (`ssu lr`), release `4.0.2.340`.

## 3.2 Заголовки, которых нет в репозитории

Установка `mesa-llvmpipe-*-devel` **конфликтует** с adaptation-пакетом (`droid-hal-*`,
libhybris). Поэтому заголовки извлекаем из RPM без установки:

```bash
B="https://releases.omprussia.ru/releases/4.0.2.340/omp/armv7hl/pj:oss:/armv7hl"
mkdir -p /home/opt
for n in mesa-llvmpipe-libEGL-devel mesa-llvmpipe-libGLESv2-devel mesa-llvmpipe-libGL-devel; do
  f="$n-21.1.6+git1-1.4.10.omp.armv7hl.rpm"
  curl -sS -O "$B/$f" && rpm2cpio "$f" | (cd /home/opt && cpio -idm --quiet)
done
# → /home/opt/usr/include/{EGL,GLES2,GLES3,GL,KHR}, /home/opt/usr/lib/pkgconfig/{egl,glesv2}.pc
```

Отсутствующий в `kernel-headers` (3.18) файл:

```bash
mkdir -p /home/opt/usr/include/linux
curl -sSL -o /home/opt/usr/include/linux/input-event-codes.h \
  https://raw.githubusercontent.com/torvalds/linux/master/include/uapi/linux/input-event-codes.h
```

Зачем: без него не собирается Wayland-часть GLFW (`KEY_*` коды).

## 3.3 Заглушки pkg-config

`/home/opt/pc/libdrm.pc` (его требует `egl.pc` в `Requires.private`):

```
prefix=/usr
Name: libdrm
Description: stub for pkg-config
Version: 2.4.100
Libs:
Cflags:
```

`/home/opt/pc/wayland-egl.pc` (в Aurora нет `wayland-egl.pc`, хотя библиотека есть):

```
prefix=/usr
includedir=${prefix}/include
libdir=${prefix}/lib
Name: wayland-egl
Description: Wayland EGL platform
Version: 1.19.0
Requires: wayland-client
Libs: -L${libdir} -lwayland-egl
Cflags: -I${includedir}
```

## 3.4 Симлинки на библиотеки

В системе только версионированные `.so.N`, линкеру нужны `.so`:

```bash
mkdir -p /home/opt/libs
ln -sf /usr/lib/libEGL.so.1             /home/opt/libs/libEGL.so
ln -sf /usr/lib/libGLESv2.so.2          /home/opt/libs/libGLESv2.so
ln -sf /usr/lib/libwayland-client.so.0  /home/opt/libs/libwayland-client.so
ln -sf /usr/lib/libwayland-cursor.so.0  /home/opt/libs/libwayland-cursor.so
ln -sf /usr/lib/libwayland-egl.so.1     /home/opt/libs/libwayland-egl.so
ln -sf /usr/lib/libxkbcommon.so.0       /home/opt/libs/libxkbcommon.so
```

## 3.5 Go (armv6l — работает на ARMv7)

```bash
curl -sSLo /tmp/go.tgz https://go.dev/dl/go1.22.12.linux-armv6l.tar.gz
tar -xzf /tmp/go.tgz -C /home/opt          # → /home/opt/go
/home/opt/go/bin/go version                # go1.22.12 linux/arm
```

## 3.6 Mesa llvmpipe (программный GL)

```bash
mkdir -p /home/opt/mesa && cd /home/opt/mesa
for n in mesa-llvmpipe-libEGL mesa-llvmpipe-libGLESv2 mesa-llvmpipe-libglapi \
         mesa-llvmpipe-libgbm mesa-llvmpipe-dri-swrast-driver libdrm; do
  f="<href из repodata>"   # см. scripts/setup-toolchain.sh — там автопоиск
  curl -sS -O "$B/$f" && rpm2cpio "$f" | cpio -idm --quiet
done
cd usr/lib && ln -sf libEGL.so.1.0.0 libEGL.so.1 \
  && ln -sf libGLESv2.so.2.0.0 libGLESv2.so.2 \
  && ln -sf libglapi.so.0.0.0 libglapi.so.0 \
  && ln -sf libgbm.so.1.0.0 libgbm.so.1 \
  && ln -sf libdrm.so.2.4.0 libdrm.so.2
```

Проверка: есть `/home/opt/mesa/usr/lib/{libEGL.so.1,libGLESv2.so.2,libgbm.so.1,libdrm.so.2}`
и `/home/opt/mesa/usr/lib/dri/swrast_dri.so`.

## 3.7 Пропатченный GLFW (для `replace`)

```bash
mkdir -p /home/opt/aurora-fyne
cp -a $(/home/opt/go/bin/go env GOMODCACHE)/github.com/go-gl/glfw/v3.3/glfw@* \
      /home/opt/aurora-fyne/go-glfw
chmod -R u+w /home/opt/aurora-fyne/go-glfw
patch -p1 -d /home/opt/aurora-fyne/go-glfw < /home/user/work/aurora-ns220/patches/glfw-wl-shell.patch
```

Альтернатива: распаковать готовый `go-glfw-aurora-patched.tgz` в `/home/opt/aurora-fyne/`
(архив лежит вне git: `~/aurora-ns220/build/fyne/go-glfw-aurora-patched.tgz`, см. `AGENTS.md`).

## 3.8 Проверка окружения

```bash
export PATH=/home/opt/go/bin:$PATH
go version
PKG_CONFIG_PATH=/home/opt/pc:/home/opt/usr/lib/pkgconfig:/usr/lib/pkgconfig \
  pkg-config --exists wayland-client wayland-cursor wayland-egl xkbcommon egl glesv2 && echo PKG_OK
ls /home/opt/usr/include/EGL/egl.h /home/opt/mesa/usr/lib/dri/swrast_dri.so
grep -c wl_shell /home/opt/aurora-fyne/go-glfw/glfw/src/wl_window.c   # >0
```

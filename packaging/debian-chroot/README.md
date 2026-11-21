# Debian chroot на планшете (Aurora 4)

Развёрнуто 2026-11-21 по `doc/11-DEBIAN-CHROOT.md`:

* rootfs: `~/aurora-ns220/...`/`/home/user/debian-armhf-rootfs.tgz` (debootstrap bookworm, armhf);
* каталог: `/home/chroot/debian` (ext4 на `/home`, требуется `mount -o remount,exec /home`);
* второй этап debootstrap выполнен на устройстве, `apt update` работает
  (`99aurora`: `APT::Sandbox::User root`, `Acquire::ForceIPv4 true`);
* установлены: mc 4.8.29, tmux 3.3a, vim 9.0, htop 3.2.2, curl 7.88.1, ncdu 1.18.

Запуск:

Установка/обновление хелпера:

```sh
gcc -O2 -o debian-enter debian-enter.c
sudo install -o root -g root debian-enter /usr/local/bin/debian-enter
sudo chmod 4755 /usr/local/bin/debian-enter      # install -m 4755 на устройстве не применяет setuid
```

Вход:

```sh
debian-chroot                 # интерактивный bash (setuid-хелпер, пароль не нужен)
debian-chroot apt-get update  # команда в chroot
deb                           # алиас
```

В меню Aurora — «Debian (chroot)» (открывает fingerterm с chroot).

Ограничение: GUI внутри chroot не работает (нет доступа к lipstick/Wayland-сессии),
только CLI. Для GUI — Fyne/Qt вне chroot.

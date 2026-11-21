/*
 * debian-enter — вход в Debian chroot без пароля (setuid root).
 *
 * Зачем: обёртка запускается из меню Aurora (fingerterm) от пользователя,
 * а bind-mount/chroot требуют root. devel-su в терминале просит пароль,
 * что неудобно; helper делает ровно одну вещь — монтирует и входит в
 * фиксированный каталог /home/chroot/debian.
 *
 * Схема:
 *   1) /home монтируется bind-ом в /run/debian-home и remount-ится с exec
 *      (сам /home в Aurora noexec, менять его флаги глобально не нужно);
 *   2) внутрь rootfs bind-ом ставятся /proc, /sys, /dev, /dev/pts и tmpfs /tmp;
 *   3) fork: ребёнок chroot + exec (по умолчанию bash -l), родитель ждёт и
 *      размонтирует всё обратно.
 *
 * ВНИМАНИЕ: это фактически беспарольный root на dev-стенде (пароль 1234 и так
 * опубликован в репозитории). Для «взрослого» использования — polkit/sailjail.
 *
 * Сборка: gcc -O2 -o debian-enter debian-enter.c
 * Установка: install -m 4755 -o root -g root debian-enter /usr/local/bin/
 */
#define _GNU_SOURCE
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <grp.h>
#include <unistd.h>

#define HOST_HOME "/home"
#define BIND_BASE "/run/debian-home"
#define ROOT_SUB  "/chroot/debian"

static const char *g_root;   /* путь к rootfs внутри bind-базы */

static void warn_mount(const char *src, const char *tgt, const char *fstype,
                       unsigned long flags, const char *data)
{
    if (mount(src, tgt, fstype, flags, data) != 0)
        fprintf(stderr, "debian-enter: mount %s -> %s: %s\n",
                src ? src : "(null)", tgt, strerror(errno));
}

static void mkdir_p(const char *path)
{
    if (mkdir(path, 0755) != 0 && errno != EEXIST)
        fprintf(stderr, "debian-enter: mkdir %s: %s\n", path, strerror(errno));
}

static void copy_file(const char *from, const char *to)
{
    FILE *in = fopen(from, "r");
    if (!in)
        return;
    FILE *out = fopen(to, "w");
    if (!out) { fclose(in); return; }
    char buf[4096];
    size_t n;
    while ((n = fread(buf, 1, sizeof buf, in)) > 0)
        fwrite(buf, 1, n, out);
    fclose(in);
    fclose(out);
}

static void umount_quiet(const char *p)
{
    if (umount(p) != 0 && umount2(p, MNT_DETACH) != 0)
        fprintf(stderr, "debian-enter: umount %s: %s\n", p, strerror(errno));
}

int main(int argc, char **argv)
{
    if (geteuid() != 0) {
        fprintf(stderr, "debian-enter: нужен setuid root (install -m 4755)\n");
        return 1;
    }

    char root[512];
    snprintf(root, sizeof root, "%s%s", BIND_BASE, ROOT_SUB);

    /* /home -> /run/debian-home, с exec */
    mkdir_p(BIND_BASE);
    if (mount(HOST_HOME, BIND_BASE, NULL, MS_BIND, NULL) != 0) {
        fprintf(stderr, "debian-enter: bind %s: %s\n", HOST_HOME, strerror(errno));
        return 1;
    }
    /* снять noexec с bind-копии (флаги /home не трогаем) */
    if (mount(NULL, BIND_BASE, NULL, MS_REMOUNT | MS_BIND, NULL) != 0)
        fprintf(stderr, "debian-enter: remount exec: %s\n", strerror(errno));

    struct stat st;
    if (stat(root, &st) != 0) {
        fprintf(stderr, "debian-enter: нет rootfs %s (см. doc/11)\n", root);
        umount_quiet(BIND_BASE);
        return 1;
    }

    char p[600];
    snprintf(p, sizeof p, "%s/tmp", root);   mkdir_p(p);
    warn_mount("tmpfs", p, "tmpfs", MS_NOSUID | MS_NODEV, "mode=1777");

    snprintf(p, sizeof p, "%s/proc", root);  mkdir_p(p);
    warn_mount("proc", p, "proc", 0, NULL);

    snprintf(p, sizeof p, "%s/sys", root);   mkdir_p(p);
    warn_mount("sysfs", p, "sysfs", 0, NULL);

    snprintf(p, sizeof p, "%s/dev", root);   mkdir_p(p);
    warn_mount("/dev", p, NULL, MS_BIND, NULL);

    snprintf(p, sizeof p, "%s/dev/pts", root); mkdir_p(p);
    warn_mount("/dev/pts", p, NULL, MS_BIND, NULL);

    char rf[600];
    snprintf(rf, sizeof rf, "%s/etc/resolv.conf", root);
    copy_file("/etc/resolv.conf", rf);

    pid_t pid = fork();
    if (pid < 0) {
        perror("debian-enter: fork");
        return 1;
    }
    if (pid == 0) {
        /* helper запущен setuid-root: сбрасываем real/effective/saved uid на 0,
           иначе внутри chroot инструменты видят безымянного uid 100000 */
        if (setgid(0) != 0 || setgroups(0, NULL) != 0 || setuid(0) != 0)
            perror("debian-enter: setuid(0)");
        if (chroot(root) != 0) { perror("debian-enter: chroot"); _exit(1); }
        if (chdir("/") != 0) { perror("debian-enter: chdir"); _exit(1); }
        setenv("HOME", "/root", 1);
        setenv("TERM", getenv("TERM") ? getenv("TERM") : "xterm-256color", 1);
        if (argc > 1)
            execvp(argv[1], &argv[1]);
        else {
            char *a[] = { "/bin/bash", "-l", NULL };
            execv(a[0], a);
        }
        perror("debian-enter: exec");
        _exit(127);
    }

    int status = 0;
    waitpid(pid, &status, 0);

    snprintf(p, sizeof p, "%s/dev/pts", root); umount_quiet(p);
    snprintf(p, sizeof p, "%s/dev", root);     umount_quiet(p);
    snprintf(p, sizeof p, "%s/sys", root);     umount_quiet(p);
    snprintf(p, sizeof p, "%s/proc", root);    umount_quiet(p);
    snprintf(p, sizeof p, "%s/tmp", root);     umount_quiet(p);
    umount_quiet(BIND_BASE);

    if (WIFEXITED(status))
        return WEXITSTATUS(status);
    return 0;
}

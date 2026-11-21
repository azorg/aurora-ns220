/*
 * aurora-sensor-reset — перезапуск sensorfwd без пароля (setuid root).
 *
 * Зачем: MTK HAL 1.3 отвергает setDelay, если сенсор остался активным после
 * предыдущего клиента; тогда sensorfwd пишет «setInterval not ok» и данные не
 * идут. Перезапуск sensorfwd возвращает сенсоры в рабочее состояние.
 * Используется приложением (если нет данных 4 с) и вручную.
 *
 * Сборка: gcc -O2 -o sensor-reset sensor-reset.c
 * Установка: install -o root -g root sensor-reset /usr/local/bin/aurora-sensor-reset
 *            chmod 4755 /usr/local/bin/aurora-sensor-reset
 */
#include <stdio.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/wait.h>

int main(int argc, char **argv)
{
    (void)argc;
    (void)argv;
    if (geteuid() != 0) {
        fprintf(stderr, "aurora-sensor-reset: нужен setuid root\n");
        return 1;
    }
    /* 1) перезапуск Android sensors HAL (сбрасывает «залипший» активный сенсор) */
    pid_t pid = fork();
    if (pid == 0) {
        execl("/system/bin/setprop", "setprop", "ctl.restart", "sensors-hal-1-0", (char *)0);
        execlp("setprop", "setprop", "ctl.restart", "sensors-hal-1-0", (char *)0);
        _exit(127);
    }
    int st = 0;
    waitpid(pid, &st, 0);
    sleep(3);

    /* 2) перезапуск sensorfwd, чтобы он переподключился к свежему HAL */
    pid = fork();
    if (pid == 0) {
        execl("/usr/bin/systemctl", "systemctl", "restart", "sensorfwd", (char *)0);
        perror("aurora-sensor-reset: systemctl");
        _exit(127);
    }
    waitpid(pid, &st, 0);
    if (WIFEXITED(st) && WEXITSTATUS(st) == 0) {
        puts("sensors HAL + sensorfwd restarted");
        return 0;
    }
    fprintf(stderr, "aurora-sensor-reset: restart failed\n");
    return 1;
}

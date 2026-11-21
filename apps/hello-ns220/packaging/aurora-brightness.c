/*
 * aurora-brightness — управление подсветкой экрана (setuid root).
 *
 * /sys/class/leds/lcd-backlight/brightness принадлежит system:system (0664),
 * пользователь в эту группу не входит, поэтому пишем через setuid-хелпер.
 *
 *   aurora-brightness          — показать текущую яркость (0..255)
 *   aurora-brightness 128      — установить
 *   aurora-brightness +10/-10  — изменить на шаг
 *
 * Сборка: gcc -O2 -o aurora-brightness aurora-brightness.c
 * Установка: install -o root -g root aurora-brightness /usr/local/bin/
 *            chmod 4755 /usr/local/bin/aurora-brightness
 */
#include <errno.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static const char *PATH = "/sys/class/leds/lcd-backlight/brightness";
static const char *MAXP = "/sys/class/leds/lcd-backlight/max_brightness";

static int read_int(const char *p, int def)
{
    FILE *f = fopen(p, "r");
    if (!f) return def;
    int v = def;
    if (fscanf(f, "%d", &v) != 1) v = def;
    fclose(f);
    return v;
}

int main(int argc, char **argv)
{
    if (geteuid() != 0) {
        fprintf(stderr, "aurora-brightness: нужен setuid root\n");
        return 1;
    }
    int max = read_int(MAXP, 255);
    if (max <= 0) max = 255;
    int cur = read_int(PATH, -1);

    if (argc < 2) {
        printf("%d %d\n", cur, max);
        return 0;
    }
    int val;
    if (argv[1][0] == '+' || argv[1][0] == '-') {
        val = (cur < 0 ? max / 2 : cur) + atoi(argv[1]);
    } else {
        val = atoi(argv[1]);
    }
    if (val < 0) val = 0;
    if (val > max) val = max;

    FILE *f = fopen(PATH, "w");
    if (!f) { perror("aurora-brightness"); return 1; }
    fprintf(f, "%d", val);
    fclose(f);
    printf("%d %d\n", val, max);
    return 0;
}

# aurora-sensor-reset — сброс сенсоров (setuid root)

MTK sensors HAL 1.3 «залипает»: после нескольких клиентских сессий
`HybrisAdaptor::setInterval` получает от HAL ошибку, и данные (ACC/компас) не идут,
хотя `QOrientationSensor` ещё отвечает. Перезапуск `sensorfwd` помогает не всегда;
надёжно — перезагрузка. Этот хелпер пробует мягкий путь без пароля:

1. `setprop ctl.restart sensors-hal-1-0` (Android init перезапускает HAL);
2. пауза 3 с;
3. `systemctl restart sensorfwd` (переподключение к свежему HAL).

Сборка/установка на устройстве:

```sh
gcc -O2 -o sensor-reset sensor-reset.c
sudo install -o root -g root sensor-reset /usr/local/bin/aurora-sensor-reset
sudo chmod 4755 /usr/local/bin/aurora-sensor-reset
```

Приложение `aurora-hello-ns220` использует его автоматически: если helper
` sensor-reader` не дал ни одного `ACC` за 5 с три раза подряд — вызывается сброс.

**Надёжный способ получить данные:** свежая перезагрузка и первое приложение-
потребитель сенсоров (см. `doc/15-PERIPHERALS.md` §15.6).

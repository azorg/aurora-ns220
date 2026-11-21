// sensor-reader — минимальный помощник: читает сенсоры через QtSensors
// (бэкенд sensorfw, D-Bus/socket-протокол которых из Go не выговорить)
// и печатает строки на stdout:
//
//   ACC <x> <y> <z> <timestamp_ms>
//   MAG <x> <y> <z> <timestamp_ms>
//   ERR <текст>
//
// Сборка на устройстве:
//   g++ -O2 -o sensor-reader sensor-reader.cpp $(pkg-config --cflags --libs Qt5Sensors Qt5Core)
//
// Зачем отдельный бинарник: QtSensors/sensorfw — единственный штатный способ
// получить G-сенсор на Aurora (прямой ioctl /dev/gsensor и hybris-HAL в
// отдельном процессе падают, см. doc/ai/bugs B031).
#include <QCoreApplication>
#include <QAccelerometer>
#include <QAccelerometerReading>
#include <QCompass>
#include <QOrientationSensor>
#include <QOrientationReading>
#include <QCompassReading>
#include <QTimer>
#include <cstdio>
#include <clocale>

int main(int argc, char** argv)
{
    QCoreApplication app(argc, argv);

    // Числа печатаем всегда с точкой: русская локаль даёт "9,071", и парсер
    // приложения (Go) такие значения не принимает.
    setlocale(LC_NUMERIC, "C");

    QAccelerometer accel;
    accel.setDataRate(10);
    QObject::connect(&accel, &QAccelerometer::readingChanged, [&]() {
        QAccelerometerReading* r = accel.reading();
        if (!r) return;
        printf("ACC %.3f %.3f %.3f %lld\n", r->x(), r->y(), r->z(),
               (long long)r->timestamp());
        fflush(stdout);
    });

    QOrientationSensor orient;
    orient.setDataRate(5);
    QObject::connect(&orient, &QOrientationSensor::readingChanged, [&]() {
        QOrientationReading* r = orient.reading();
        if (!r) return;
        printf("ORT %d %lld\n", (int)r->orientation(), (long long)r->timestamp());
        fflush(stdout);
    });
    orient.start();

    QCompass compass;
    compass.setDataRate(2);
    QObject::connect(&compass, &QCompass::readingChanged, [&]() {
        QCompassReading* r = compass.reading();
        if (!r) return;
        printf("MAG %.3f %.3f %.3f %lld\n", r->azimuth(), r->calibrationLevel(), 0.0,
               (long long)r->timestamp());
        fflush(stdout);
    });

    bool ok = accel.start();
    printf("START accel=%d active=%d error=%d\n", (int)ok, (int)accel.isActive(), (int)accel.error());
    fflush(stdout);
    compass.start();
    // Опрос: если HAL не присылает readingChanged, currentReading всё равно
    // может обновляться; печатаем раз в 300 мс для диагностики.
    auto* poll = new QTimer(&app);
    QObject::connect(poll, &QTimer::timeout, [&]() {
        QAccelerometerReading* r = accel.reading();
        if (!r) {
            printf("POLL no reading active=%d err=%d\n", (int)accel.isActive(), (int)accel.error());
        } else {
            printf("POLL %.3f %.3f %.3f err=%d\n", r->x(), r->y(), r->z(), (int)accel.error());
        }
        fflush(stdout);
    });
    poll->start(300);

    // Остановка по SIGTERM/SIGINT — QCoreApplication завершится.
    QObject::connect(&app, &QCoreApplication::aboutToQuit, [&]() {
        accel.stop();
        compass.stop();
    });
    QTimer::singleShot(0, [&]() { printf("READY\n"); fflush(stdout); });
    return app.exec();
}

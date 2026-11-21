# Как выглядит OSK (сборка под Linux, Xvfb 1200×1920)

Снято 2026-11-21 на Linux-хосте (`go build -tags gui`, Xvfb + xfwm4, окно 1200×1894,
`FYNE_SCALE=1.0`, шрифт DejaVu Sans Mono 16).

| Файл | Что видно |
|---|---|
| `01-states.png` | 6 состояний: EN, Shift (заливка 50%), RU (ЙЦУКЕН), ?123, Ctrl (cyan 50%), свёрнутый ряд |
| `02-full-window-en.png` | окно целиком, EN |
| `03-full-window-ru.png` | окно целиком, RU |
| `04-collapsed.png` | свёрнуто: Esc Tab Пробел ← ↑ ↓ → Ctrl+C ⏎ + жёлтая «Развернуть ▴» |
| `05-zoom-ctrl-oneshot.png` | крупно: Ctrl в режиме ONE-SHOT (половинная заливка cyan) |

Замечания прогона:
* клики по клавишам (`xdotool`) доходят до PTY: набрано `qwe `;
* статус-строка снизу показывает состояние: `EN`, `EN ·Shift(1)`, `EN ·Ctrl(1)`.

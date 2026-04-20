# Интерактивный музей — веб-приложение

Веб-приложение с компьютерным зрением для определения музейных экспонатов по фото с камеры. Может работать как обычный сайт и как mini-app внутри MAX через MAX Bridge.

## Быстрый старт (Docker)

Все сервисы запускаются одной командой:

```bash
docker-compose up --build -d
```

После запуска:
- **Бот**: работает как обычно (Max Bot API)
- **Web Backend**: http://localhost:8000
- **Web Frontend**: http://localhost:8080
- **База данных**: localhost:5432

### Настройка WEB_APP_URL для бота

В `.env` файле бота укажите:

```env
# Для доступа из MAX нужен публичный HTTPS URL:
WEB_APP_URL=https://abc123.ngrok-free.app
```

`WEB_APP_URL` должен быть публичным `https://` адресом. Локальные адреса вида `http://localhost` или `http://127.0.0.1` подходят только для локального браузера, но не для mini-app кнопок в MAX.

### Ngrok для доступа из Max

```bash
ngrok http 8080
```

Скопируйте HTTPS URL в `.env` → `WEB_APP_URL` → перезапустите бота.

---

## Разработка (hot reload)

Для разработки с автоперезагрузкой:

```bash
docker-compose -f docker-compose.yml -f docker-compose.override.yml up -d
```

Или отдельно:
```bash
# Только БД и бот
docker-compose up -d db app

# Backend вручную (с hot reload)
cd web_app/backend
pip install -r requirements.txt
uvicorn main:app --reload

# Frontend вручную (с hot reload)
cd web_app/frontend
npm install
npm run dev
```

---

## Архитектура

```
Пользователь → MAX Bot / Browser → React + MAX Bridge → FastAPI → EfficientNetB0 → результат
                                                   ↓
                                              PostgreSQL (музеи, staff, экспонаты)
```

## Структура

```
web_app/
├── backend/
│   ├── main.py              # FastAPI приложение
│   ├── db.py                # Подключение к БД
│   ├── routers/
│   │   ├── museums.py       # GET /api/museums/...
│   │   ├── recognition.py   # POST /api/recognition/identify
│   │   └── admin.py         # Админ-панель API
│   ├── ml/
│   │   ├── model.py         # Загрузка и инференс моделей
│   │   └── train.py         # Обучение EfficientNetB0 per-museum
│   ├── models/              # .pt файлы моделей (создаются при обучении)
│   └── requirements.txt
└── frontend/
    └── src/
        ├── App.jsx
        ├── api.js
        ├── hooks/
        │   └── useMaxBridge.js
        └── components/
            ├── MuseumSelect.jsx   # Список музеев + геолокация
            ├── MuseumMap.jsx      # Яндекс Карта с музеями
            ├── MuseumDetail.jsx   # Страница музея (описание, выставки)
            ├── CameraView.jsx     # Камера + распознавание
            ├── ExhibitResult.jsx  # Результат распознавания
            └── AdminPanel.jsx     # Панель администратора
```

## Запуск

### 1. Backend

```bash
cd web_app/backend
pip install -r requirements.txt
uvicorn main:app --reload
```

### 2. Frontend

```bash
cd web_app/frontend
npm install
npm run dev
```

Локально: http://localhost:5173

### 3. Ngrok (чтобы открыть из Max на телефоне)

MAX на телефоне не может открыть `localhost`. Нужно пробросить порт в интернет и указать публичный адрес в `WEB_APP_URL`:

**Установка ngrok:** https://dashboard.ngrok.com/get-started/setup

```bash
# Запускаем ngrok на порту фронтенда (5173)
ngrok http 5173
```

Получите URL вида `https://abc123.ngrok-free.app` — это ваш публичный адрес.

**В `.env` бота:**
```
WEB_APP_URL=https://abc123.ngrok-free.app
```

Перезапустите бота. Теперь кнопки в MAX откроют ваше локальное приложение как mini-app.

> 💡 ngrok URL меняется при каждом перезапуске. Для постоянного URL нужен тариф или домен.

## MAX mini-app и авторизация

- Бот открывает приложение через `open_app`
- Если кнопка привязана к музею, его id передаётся через `payload/start_param`
- В браузере страница музея может открываться по `/?museum_id=<id>`
- Внутри MAX приложение получает `window.WebApp.initData`, `initDataUnsafe.user` и `start_param`
- Backend валидирует `initData` по `BOT_TOKEN`

### Вход в админку

- **Обычный браузер** — `email + MAX ID + OTP`
- **MAX mini-app** — валидированный `initData + OTP`

Для mini-app логина сотрудник должен существовать в таблице `staff`, а `staff.user_id` должен совпадать с MAX user id.

## Обучение модели для музея

Необходимо минимум 2 экспоната с загруженными фото в боте.

```bash
cd web_app/backend
python -m ml.train --museum_id 1
```

Или через API (из панели администратора):
```
POST /api/recognition/train/{museum_id}
```

### Стратегия обучения (EfficientNetB0)

1. **Загрузка данных**: фото берутся из `image_url` в таблице `exhibits`
2. **Аугментация**: каждое фото расширяется до 20 вариантов (flip, rotate, color jitter)
3. **Этап 1** (5 эпох): обучается только классификатор, backbone заморожен
4. **Этап 2** (10 эпох): полный fine-tuning с AdamW + CosineAnnealingLR
5. Сохраняется `models/museum_{id}.pt` + `museum_{id}_meta.json`

### Рекомендации по качеству

| Фото на экспонат | Ожидаемая точность |
|---|---|
| 1-2 | ~50-60% (работает за счёт аугментации) |
| 5-10 | ~75-85% |
| 20+ | ~90%+ |

- Снимайте под разными углами, при разном освещении
- Чем больше уникальных экспонатов, тем важнее больше фото
- Фото загружаются через бота командой `📷 Добавить фото` в управлении экспонатом

## Структура URL

| URL | Описание |
|-----|----------|
| `/` | Выбор музея (список / карта) |
| `/?museum_id=1` | Сразу открывает страницу музея с id=1 |

В MAX mini-app бот передаёт музей через `start_param`, а `?museum_id=` остаётся браузерным fallback.

## API

| Метод | URL | Описание |
|---|---|---|
| GET | /api/museums/ | Список всех музеев |
| GET | /api/museums/{id} | Детальная информация о музее |
| GET | /api/museums/{id}/exhibitions | Выставки музея |
| GET | /api/museums/{id}/exhibits | Экспонаты музея |
| GET | /api/museums/nearby?lat=&lon= | Ближайшие музеи |
| GET | /api/museums/{id}/model-status | Статус модели |
| POST | /api/recognition/identify | Распознать экспонат (multipart) |
| POST | /api/recognition/train/{id} | Запустить обучение |

## Troubleshooting

- **«Модель не найдена»** — не обучена. Запустите `python -m ml.train --museum_id X`
- **Нет имени в шапке / открывается как обычный сайт** — проверьте, что приложение открыто именно из кнопки бота в MAX, а `WEB_APP_URL` указывает на актуальный публичный `https://` адрес
- **«Не удалось открыть камеру»** — разрешите доступ к камере в MAX или браузере; на телефоне используйте fallback `Открыть камеру телефона`
- **Ngrok 502 Bad Gateway** — убедитесь что React (5173) и FastAPI (8000) запущены
- **OTP не приходит в mini-app** — проверьте `BOT_TOKEN`, SMTP-настройки и наличие сотрудника в `staff` с правильным `user_id`
- **URL ngrok меняется** — нормально для бесплатного тарифа, обновите `WEB_APP_URL` в `.env`

# SK Museum Bot

Max-бот и веб-приложение для музеев Ставропольского края: каталог, геолокация, поиск, распознавание экспонатов по фото (EfficientNetB0).

## Возможности

**Для посетителей:**
- Каталог 47 музеев с поиском, пагинацией и геолокацией
- Интерактивная Яндекс Карта с музеями
- Страница музея с описанием, контактами и списком выставок
- Распознавание экспонатов по фото с камеры (компьютерное зрение)
- Мероприятия с записью, отзывы и оценки

**Для сотрудников:**
- 2FA через email, RBAC (4 роли)
- Веб-панель администратора: управление экспонатами, загрузка фото, обучение моделей
- Аналитика посещаемости и поисковых запросов

## Архитектура

```
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│  Max Bot API │   │  Go Backend  │   │ PostgreSQL 15│
│              │◄─►│              │◄─►│              │
└──────────────┘   └──────────────┘   └──────┬───────┘
                                             │
┌──────────────┐   ┌──────────────┐          │
│ React + Vite │◄─►│ FastAPI +    │◄─────────┘
│ (Frontend)   │   │ PyTorch (ML) │
└──────────────┘   └──────────────┘
```

## Быстрый старт

### Требования

- **Docker & Docker Compose** (рекомендуется)
- Go 1.26+, Node.js 20+, Python 3.11+ (для локальной разработки)

### 1. Клонирование и настройка

```bash
git clone <repository-url>
cd sk-museum-bot
cp .env.example .env
# Отредактируйте .env — укажите реальные значения
```

### 2. Yandex Maps API Key

Получите бесплатный ключ на [developer.tech.yandex.ru](https://developer.tech.yandex.ru/) и вставьте в `web_app/frontend/index.html` вместо `YOUR_YANDEX_MAPS_API_KEY`.

### 3. Запуск через Docker

```bash
docker-compose up --build -d
```

После запуска:
- **Бот** — работает через Max Bot API
- **Web Frontend** — http://localhost:8080
- **Web Backend API** — http://localhost:8000
- **PostgreSQL** — localhost:5432

БД инициализируется автоматически из `init.sql` при первом запуске.

## Структура проекта

```
sk-museum-bot/
├── main.go                  # Max-бот (Go) — точка входа
├── handlers.go              # Обработчики сообщений бота
├── admin_functions.go       # Админские функции бота
├── user_functions.go        # Пользовательские функции бота
├── exhibition_functions.go  # Работа с выставками
├── init.sql                 # Схема БД + тестовые данные (47 музеев)
├── docker-compose.yml       # Запуск всех сервисов
├── Dockerfile               # Сборка Go-бота
├── .env.example             # Шаблон переменных окружения
│
└── web_app/                 # Веб-приложение
    ├── backend/             # FastAPI + ML
    │   ├── main.py          # Точка входа API
    │   ├── db.py            # Подключение к PostgreSQL
    │   ├── routers/
    │   │   ├── museums.py   # /api/museums/*
    │   │   ├── recognition.py # /api/recognition/*
    │   │   └── admin.py     # /api/admin/*
    │   ├── ml/
    │   │   ├── model.py     # Инференс EfficientNetB0
    │   │   └── train.py     # Обучение моделей
    │   └── models/          # .pt файлы (gitignored)
    │
    └── frontend/            # React + Vite + Tailwind
        └── src/
            ├── App.jsx
            ├── api.js
            └── components/
                ├── MuseumSelect.jsx  # Список музеев + геолокация
                ├── MuseumMap.jsx     # Яндекс Карта с музеями
                ├── MuseumDetail.jsx  # Страница музея (инфо, выставки)
                ├── CameraView.jsx   # Камера + распознавание
                ├── ExhibitResult.jsx # Результат распознавания
                └── AdminPanel.jsx   # Панель администратора
```

## База данных

- **museums** — 47 музеев с координатами, контактами
- **exhibitions** — выставки (привязаны к музеям)
- **exhibits** — экспонаты (привязаны к выставкам)
- **exhibit_categories** — категории экспонатов
- **events** — мероприятия
- **reviews** — отзывы, **staff** — сотрудники с ролями

## Web API

| Метод | URL | Описание |
|-------|-----|----------|
| GET | `/api/museums/` | Список всех музеев |
| GET | `/api/museums/{id}` | Детальная информация о музее |
| GET | `/api/museums/{id}/exhibitions` | Выставки музея |
| GET | `/api/museums/{id}/exhibits` | Экспонаты музея |
| GET | `/api/museums/nearby?lat=&lon=` | Ближайшие музеи |
| GET | `/api/museums/{id}/model-status` | Статус ML-модели |
| POST | `/api/recognition/identify` | Распознать экспонат (multipart) |
| POST | `/api/recognition/train/{id}` | Запустить обучение модели |

## Локальная разработка

```bash
# БД
docker-compose up -d db

# Backend
cd web_app/backend
pip install -r requirements.txt
uvicorn main:app --reload

# Frontend
cd web_app/frontend
npm install
npm run dev
```

Для доступа из Max (мобильный):

```bash
ngrok http 5173
# Скопируйте HTTPS URL → .env → WEB_APP_URL
```

## Переменные окружения

См. `.env.example`. Обязательные:

| Переменная | Описание |
|------------|----------|
| `DATABASE_URL` | PostgreSQL connection string |
| `BOT_TOKEN` | Токен Max Bot |
| `SMTP_USER`, `SMTP_PASS` | Email для 2FA |
| `YANDEX_MAPS_API_KEY` | Ключ Яндекс Карт |

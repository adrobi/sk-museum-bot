# SK Museum Bot

MAX-бот и веб-приложение для музеев Ставропольского края: каталог, геолокация, поиск, распознавание экспонатов по фото (EfficientNetB0). Веб-приложение работает и как обычный сайт, и как mini-app внутри MAX через MAX Bridge.

## Краткая инструкция по запуску SK Museum Bot

Проект состоит из трех основных частей: Go-бота, Python-бэкенда с нейросетью и React-фронтенда. Все они связаны через базу данных PostgreSQL.

#### 1. Подготовка окружения
Убедитесь, что у вас установлены **Docker** и **Docker Compose**.

1. Клонируйте проект и перейдите в его директорию.
2. Создайте файл конфигурации из шаблона:
   ```bash
   cp .env.example .env
   ```
3. Откройте `.env` и заполните обязательные параметры:
   * `BOT_TOKEN`: токен вашего бота в MAX.
   * `WEB_APP_URL`: для работы кнопок в боте нужен HTTPS адрес (например, от `ngrok`).
   * `BOOTSTRAP_BOT_ADMIN_EMAIL`: email главного администратора.
   * `BOOTSTRAP_BOT_ADMIN_MAX_ID`: MAX ID главного администратора.
   * `DATABASE_URL`: настройки подключения к БД (по умолчанию для Docker уже настроены).
   * `YOUR_YANDEX_MAPS_API_KEY`: ключ для Яндекс Карт (Кабинет разработчика Яндекса – ключ для «JavaScript API и HTTP Геокодер», также нужно вставить в `web_app/frontend/index.html`).

#### 2. Запуск системы
Запустите все сервисы одной командой:
```bash
docker-compose up --build -d
```
Это поднимет базу данных, бота, бэкенд и фронтенд в изолированных контейнерах.

#### 4. Доступ к приложению

В зависимости от ваших целей, используйте разные способы доступа:

* **Для локальной разработки в браузере:**
    Перейдите по адресу **`http://localhost:5173`**. Здесь вы можете проверять верстку, работу карты и каталога без мессенджера.
    
* **Для тестирования Mini-App внутри MAX:**
    Предлагается туннель через ngrok. Возможны и другие методы, главное чтобы https.
    *Это необходимо, так как система безопасности бота блокирует ссылки на localhost для защиты данных пользователей.*

#### 5. Первичная настройка нейросети
После запуска база данных заполнится 47 музеями из файла init.sql. Таблица `staff` стартует пустой, а главный администратор создаётся ботом из переменных `BOOTSTRAP_BOT_ADMIN_EMAIL` и `BOOTSTRAP_BOT_ADMIN_MAX_ID` (только если в `staff` нет `bot_admin`). Чтобы заработало распознавание экспонатов нужно зайти в веб-приложение (или mini-app) через главного админа, тогда станет возможно обучить модель для каждого музея (до 20 фото на экспонат, разметка данных не требуется).

## Возможности

**Для посетителей:**
- Каталог 47 музеев с поиском, пагинацией и геолокацией
- Интерактивная Яндекс Карта с музеями
- Страница музея с описанием, контактами и списком выставок
- Открытие музея из бота сразу внутри MAX mini-app через `open_app` и `start_param`
- Распознавание экспонатов по фото с камеры, включая fallback на системную камеру телефона
- Мероприятия с записью, отзывы и оценки

**Для сотрудников:**
- 2FA через email, RBAC (4 роли)
- Вход в админку в браузере по `email + MAX ID + OTP`
- Вход в админку в MAX mini-app по валидированному `initData + OTP`
- Веб-панель администратора: управление экспонатами, загрузка фото, обучение моделей
- Аналитика посещаемости и поисковых запросов

## Архитектура

```text
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

### 2. Настройка переменных окружения

Минимально заполните в `.env`:

- `BOT_TOKEN` — используется и ботом, и backend веб-приложения для валидации `window.WebApp.initData`
- `SMTP_USER`, `SMTP_PASS`, `SMTP_HOST`, `SMTP_PORT` — отправка OTP-кодов сотрудникам
- `WEB_APP_URL` — публичный `https://` URL веб-приложения для кнопок mini-app в MAX
- `BOOTSTRAP_BOT_ADMIN_EMAIL` — email главного администратора для первичного bootstrap
- `BOOTSTRAP_BOT_ADMIN_MAX_ID` — MAX ID главного администратора для первичного bootstrap

Если `WEB_APP_URL` пустой, локальный или не `https`, бот не будет добавлять кнопки mini-app в меню.
Если в `staff` нет `bot_admin`, бот при старте попробует создать главного администратора из `BOOTSTRAP_BOT_ADMIN_EMAIL` и `BOOTSTRAP_BOT_ADMIN_MAX_ID`.

### 3. Yandex Maps API Key

Получите бесплатный ключ на [developer.tech.yandex.ru](https://developer.tech.yandex.ru/) и вставьте в `web_app/frontend/index.html` вместо `YOUR_YANDEX_MAPS_API_KEY`.

### 4. Запуск через Docker

```bash
docker-compose up --build -d
```

После запуска:
- **Бот** — работает через Max Bot API
- **Web Frontend** — http://localhost:8080
- **Web Backend API** — http://localhost:8000
- **PostgreSQL** — localhost:5432

БД инициализируется автоматически из `init.sql` при первом запуске.
`init.sql` заполняет музеи и витринные данные, но не создаёт пользователей `staff`.

## MAX mini-app

- Бот открывает веб-приложение через `open_app`, а `museum_id` передаётся через `payload/start_param`
- Для прямого открытия в браузере по-прежнему поддерживается fallback `?museum_id=<id>`
- `WEB_APP_URL` должен быть публичным `https://` адресом и mini-app должен быть привязан к этому же боту в MAX
- Backend веб-приложения проверяет `initData` по `BOT_TOKEN`, поэтому токен должен совпадать с токеном бота

Для локального тестирования с телефона:

```bash
# если фронтенд запущен через Docker на 8080
ngrok http 8080

# если фронтенд запущен через Vite на 5173
ngrok http 5173
```

Скопируйте выданный `https://...` URL в `.env` → `WEB_APP_URL` и перезапустите бота.

## Структура проекта

```text
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
            ├── hooks/
            │   └── useMaxBridge.js
            └── components/
                ├── MuseumSelect.jsx   # Список музеев + геолокация
                ├── MuseumMap.jsx      # Яндекс Карта с музеями
                ├── MuseumDetail.jsx   # Страница музея (инфо, выставки)
                ├── CameraView.jsx     # Камера + распознавание
                ├── ExhibitResult.jsx  # Результат распознавания
                └── AdminPanel.jsx     # Панель администратора
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
| POST | `/api/admin/login` | Отправить OTP для входа в админку |
| POST | `/api/admin/verify` | Подтвердить OTP и получить сессию |
| GET | `/api/admin/museums` | Получить музеи, доступные сотруднику |

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

Для доступа из MAX на телефоне:

```bash
# Vite dev server
ngrok http 5173

# или Docker frontend
ngrok http 8080
```

Скопируйте HTTPS URL → `.env` → `WEB_APP_URL`.

## Переменные окружения

См. `.env.example`. Обязательные:

| Переменная | Описание |
|------------|----------|
| `DATABASE_URL` | PostgreSQL connection string |
| `BOT_TOKEN` | Токен MAX Bot, также используется backend для валидации `initData` |
| `SMTP_USER`, `SMTP_PASS` | Email для 2FA |
| `WEB_APP_URL` | Публичный `https://` URL mini-app для кнопок бота |
| `BOOTSTRAP_BOT_ADMIN_EMAIL` | Email главного администратора для автосоздания при пустом `staff` |
| `BOOTSTRAP_BOT_ADMIN_MAX_ID` | MAX ID главного администратора для автосоздания при пустом `staff` |
| `YANDEX_MAPS_API_KEY` | Ключ Яндекс Карт |

## Команда проекта

* **Алексей Дробин** — Проектирование архитектуры, full-stack разработка (Go, React, FastAPI), интеграция нейросетевых моделей (EfficientNetB0) и реализация MAX-бота.
* **Софья Нафталиева** — Глубокий анализ предметной области, сбор данных по музейному сектору, формализация бизнес-требований и проектирование пользовательских сценариев специально для платформы MAX.
* **Анастасия Финченко** — Проектирование пользовательского опыта и интерфейсов mini-app, создание интерактивных прототипов и разработка единой дизайн-системы веб-приложения.
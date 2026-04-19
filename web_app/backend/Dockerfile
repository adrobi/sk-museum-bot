FROM python:3.11-slim

WORKDIR /app

# Только то что нужно asyncpg для компиляции
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# Копирование и установка зависимостей (с кэшированием слоя)
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

# Копирование кода
COPY . .

# Создание директории для моделей
RUN mkdir -p models

EXPOSE 8000

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]

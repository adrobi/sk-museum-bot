@echo off
chcp 65001 >nul
echo ===========================================
echo  Интерактивный музей - Ngrok launcher
echo ===========================================
echo.
echo Этот скрипт запускает ngrok для доступа из Max
echo.

REM Проверка наличия ngrok
where ngrok >nul 2>&1
if errorlevel 1 (
    echo [ERROR] ngrok не найден в PATH
    echo.
    echo Установка:
    echo 1. Скачайте: https://dashboard.ngrok.com/get-started/setup
    echo 2. Распакуйте и добавьте в PATH
    echo.
    pause
    exit /b 1
)

echo [1] Проверка ngrok auth...
ngrok version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] ngrok не настроен. Запустите: ngrok config add-authtoken YOUR_TOKEN
    pause
    exit /b 1
)

echo [OK] ngrok готов
echo.
echo [2] Запуск ngrok на порту 5173 (React frontend)...
echo.
echo URL будет показан ниже. Обновите WEB_APP_URL в .env бота!
echo.
echo ===========================================
ngrok http 5173

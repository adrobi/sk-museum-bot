package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

const (
	RoleBotAdmin       = "bot_admin"
	RoleMuseumAdmin    = "museum_admin"
	RoleContentManager = "content_manager"
	RoleAnalyst        = "analyst"
)

var webAppURL string

func getWebAppURL() string {
	if webAppURL != "" {
		return webAppURL
	}
	webAppURL = strings.TrimSpace(os.Getenv("WEB_APP_URL"))
	return webAppURL
}

func getPublicWebAppURL() (string, bool) {
	raw := getWebAppURL()
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	if u.Scheme != "https" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "", false
	}
	return u.String(), true
}

func getBrowserWebAppURL() (string, bool) {
	raw := getWebAppURL()
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	return u.String(), true
}

func getMuseumWebAppURL(museumId int64) (string, bool) {
	_ = museumId
	return getPublicWebAppURL()
}

func getMuseumWebBrowserURL(museumId int64) (string, bool) {
	baseURL, ok := getBrowserWebAppURL()
	if !ok {
		return "", false
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", false
	}
	q := u.Query()
	q.Set("museum_id", strconv.FormatInt(museumId, 10))
	u.RawQuery = q.Encode()
	return u.String(), true
}

type UserState struct {
	State     string
	ExtraData string
	UpdatedAt time.Time
}
type UserSession struct {
	Role      string
	UserId    int64
	LoginAt   time.Time
	ExpiresAt time.Time
}

var (
	userStates   = make(map[int64]*UserState)
	userSessions = make(map[int64]*UserSession)
	statesMu     sync.RWMutex
	sessionsMu   sync.RWMutex
)

func getUserState(userId int64) *UserState {
	statesMu.RLock()
	defer statesMu.RUnlock()
	return userStates[userId]
}
func setUserState(userId int64, state, extra string) {
	statesMu.Lock()
	defer statesMu.Unlock()
	userStates[userId] = &UserState{State: state, ExtraData: extra, UpdatedAt: time.Now()}
}
func clearUserState(userId int64) {
	statesMu.Lock()
	defer statesMu.Unlock()
	delete(userStates, userId)
}
func getSession(userId int64) *UserSession {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	s := userSessions[userId]
	if s != nil && time.Now().After(s.ExpiresAt) {
		return nil
	}
	return s
}
func setSession(userId int64, role string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	userSessions[userId] = &UserSession{Role: role, UserId: userId, LoginAt: time.Now(), ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}
}
func clearSession(userId int64) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	delete(userSessions, userId)
}

func canAccessMuseum(ctx context.Context, pool *pgxpool.Pool, userId int64, role string, museumId int64) bool {
	if role == RoleBotAdmin {
		return true
	}
	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM staff WHERE user_id=$1 AND museum_id=$2", userId, museumId).Scan(&count)
	return count > 0
}

func getStaffMuseumIds(ctx context.Context, pool *pgxpool.Pool, userId int64) []int64 {
	rows, err := pool.Query(ctx, "SELECT museum_id FROM staff WHERE user_id=$1 AND museum_id IS NOT NULL", userId)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// answerCb обновляет сообщение на месте (через callback) или отправляет новое
func answerCb(ctx context.Context, api *maxbot.Api, chatId int64, cbId, text string, kb *maxbot.Keyboard) {
	if cbId != "" {
		body := &schemes.NewMessageBody{Text: text, Attachments: []interface{}{}}
		if kb != nil {
			body.Attachments = append(body.Attachments, schemes.NewInlineKeyboardAttachmentRequest(kb.Build()))
		}
		if _, err := api.Messages.AnswerOnCallback(ctx, cbId, &schemes.CallbackAnswer{Message: body}); err == nil {
			return
		} else {
			log.Printf("answerCb callback error: %v", err)
		}
		log.Printf("answerCb fallback: send new msg")
	}
	msg := maxbot.NewMessage().SetChat(chatId).SetText(text)
	if kb != nil {
		msg.AddKeyboard(kb)
	}
	if err := api.Messages.Send(ctx, msg); err != nil {
		log.Printf("answerCb send error: %v", err)
	}
}

func setupBotCommands(ctx context.Context, api *maxbot.Api) {
	_, err := api.Bots.PatchBot(ctx, &schemes.BotPatch{
		Commands: []schemes.BotCommand{
			{Name: "start", Description: "Открыть главное меню"},
			{Name: "myid", Description: "Показать мой MAX ID"},
		},
	})
	if err != nil {
		log.Printf("setup bot commands error: %v", err)
	}
}

func ensureMainAdminFromEnv(ctx context.Context, pool *pgxpool.Pool) {
	const maxRetries = 10
	var botAdminCount int
	var err error
	for i := 0; i < maxRetries; i++ {
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM staff WHERE role='bot_admin'").Scan(&botAdminCount)
		if err == nil {
			break
		}
		log.Printf("ensure main admin: DB not ready, retry %d/%d: %v", i+1, maxRetries, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Printf("ensure main admin: failed to count bot_admin users after %d retries: %v", maxRetries, err)
		return
	}
	if botAdminCount > 0 {
		return
	}

	adminEmail := strings.TrimSpace(os.Getenv("BOOTSTRAP_BOT_ADMIN_EMAIL"))
	adminIDRaw := strings.TrimSpace(os.Getenv("BOOTSTRAP_BOT_ADMIN_MAX_ID"))
	if adminEmail == "" || adminIDRaw == "" {
		log.Printf("ensure main admin: staff has no bot_admin; set BOOTSTRAP_BOT_ADMIN_EMAIL and BOOTSTRAP_BOT_ADMIN_MAX_ID")
		return
	}

	adminID, err := strconv.ParseInt(adminIDRaw, 10, 64)
	if err != nil || adminID <= 0 {
		log.Printf("ensure main admin: invalid BOOTSTRAP_BOT_ADMIN_MAX_ID=%q", adminIDRaw)
		return
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO staff (user_id, email, role, alias, museum_id, is_active)
		 VALUES ($1, $2, 'bot_admin', $3, NULL, true)
		 ON CONFLICT (user_id) DO UPDATE SET
		   email = EXCLUDED.email,
		   role = EXCLUDED.role,
		   alias = EXCLUDED.alias,
		   museum_id = NULL,
		   is_active = true`,
		adminID,
		adminEmail,
		"Главный администратор",
	)
	if err != nil {
		log.Printf("ensure main admin: upsert failed: %v", err)
		return
	}

	log.Printf("ensure main admin: created bot_admin for user_id=%d", adminID)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env не найден")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("БД:", err)
	}
	defer pool.Close()
	ensureMainAdminFromEnv(ctx, pool)
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN не установлен")
	}
	api, err := maxbot.New(token)
	if err != nil {
		log.Fatal("API:", err)
	}
	setupBotCommands(ctx, api)
	fmt.Println("Бот запущен.")
	for update := range api.GetUpdates(ctx) {
		switch upd := update.(type) {
		case *schemes.BotStartedUpdate:
			go sendMainMenu(ctx, api, upd.GetChatID(), "")
		case *schemes.MessageCreatedUpdate:
			go handleMessage(ctx, api, pool, upd)
		case *schemes.MessageCallbackUpdate:
			go handleCallback(ctx, api, pool, upd)
		}
	}
}

func handleMessage(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, upd *schemes.MessageCreatedUpdate) {
	chatId := upd.GetChatID()
	userId := upd.GetUserID()
	text := strings.TrimSpace(upd.GetText())
	if text == "/myid" {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText(fmt.Sprintf("🆔 Ваш ID: %d", userId)))
		return
	}
	if strings.HasPrefix(text, "/train ") || text == "/train" {
		session := getSession(userId)
		if session == nil || (session.Role != RoleBotAdmin && session.Role != RoleMuseumAdmin) {
			_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("⛔ Только для администраторов."))
			return
		}
		// /train <museum_id>
		parts := strings.Split(text, " ")
		if len(parts) < 2 {
			_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("Использование: /train <museum_id>"))
			return
		}
		mid, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ museum_id должен быть числом."))
			return
		}
		// Проверяем доступ
		if session.Role != RoleBotAdmin && !canAccessMuseum(ctx, pool, userId, session.Role, mid) {
			_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("⛔ Нет доступа к этому музею."))
			return
		}
		// Асинхронно запускаем обучение
		go func() {
			_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText(
				fmt.Sprintf("🤖 Запуск обучения модели для музея %d...\nЭто займет 2-5 минут.", mid)))
			cmd := fmt.Sprintf("python -m ml.train --museum_id %d", mid)
			// Простой запуск без ожидания результата (можно улучшить через polling)
			_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText(
				"📋 Для запуска выполните в папке web_app/backend:\n\n"+cmd+"\n\nПосле обучения кнопка 🔬 в боте будет работать!"))
		}()
		return
	}
	if text == "/cancel" {
		clearUserState(userId)
		sendMainMenu(ctx, api, chatId, "")
		return
	}
	if len(text) == 6 {
		if code, err := strconv.Atoi(text); err == nil {
			verify2FA(ctx, api, pool, chatId, userId, code)
			return
		}
	}
	if len(upd.Message.Body.Attachments) > 0 {
		for _, att := range upd.Message.Body.Attachments {
			if locAtt, ok := att.(*schemes.LocationAttachment); ok {
				handleLocation(ctx, api, pool, chatId, locAtt.Latitude, locAtt.Longitude)
				return
			}
			if photoAtt, ok := att.(*schemes.PhotoAttachment); ok {
				state := getUserState(userId)
				if state != nil && strings.HasSuffix(state.State, "_photo") {
					handlePhotoUpload(ctx, api, pool, chatId, userId, state, photoAtt)
					return
				}
			}
		}
	}
	state := getUserState(userId)
	if state != nil {
		switch state.State {
		case "awaiting_museum_data":
			handleAddMuseumData(ctx, api, pool, chatId, userId, text)
			return
		case "awaiting_museum_edit":
			handleEditMuseumData(ctx, api, pool, chatId, userId, text, state.ExtraData)
			return
		case "awaiting_exhibition_data":
			handleAddExhibitionData(ctx, api, pool, chatId, userId, text, state.ExtraData)
			return
		case "awaiting_exhibit_data":
			handleAddExhibitData(ctx, api, pool, chatId, userId, text, state.ExtraData)
			return
		case "awaiting_event_data":
			handleAddEventData(ctx, api, pool, chatId, userId, text, state.ExtraData)
			return
		case "awaiting_staff_data":
			handleAddStaffData(ctx, api, pool, chatId, userId, text)
			return
		case "awaiting_staff_edit":
			handleEditStaffData(ctx, api, pool, chatId, userId, text, state.ExtraData)
			return
		case "awaiting_review_text":
			handleReviewText(ctx, api, pool, chatId, userId, text, state.ExtraData)
			return
		case "awaiting_museum_photo", "awaiting_exhibition_photo", "awaiting_exhibit_photo":
			_ = api.Messages.Send(ctx,
				maxbot.NewMessage().
					SetChat(chatId).
					AddKeyboard(photoCancelKeyboard(api)).
					SetText("📷 Сейчас ожидается фотография\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте фото или нажмите «❌ Отмена»."),
			)
			return
		}
	}
	switch text {
	case "/start", "Главное меню", "🏠 Меню", "🏠 Главное меню":
		sendMainMenu(ctx, api, chatId, "")
		return
	case "🏛 Список музеев":
		showMuseums(ctx, api, pool, chatId, 0, "")
		return
	case "📍 Ближайшие":
		kb := api.Messages.NewKeyboardBuilder()
		kb.AddRow().AddGeolocation("📍 Отправить геолокацию", true)
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText("📍 Отправьте геолокацию:"))
		return
	case "🔍 Поиск музеев":
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("🔍 Поиск музеев\n━━━━━━━━━━━━━━━━━━━━\n\nВведите название или ключевые слова:"))
		return
	case "📅 Мероприятия":
		showEvents(ctx, api, pool, chatId)
		return
	case "👨‍💻 Вход для сотрудников":
		startAuthProcess(ctx, api, pool, chatId, userId)
		return
	}
	if text != "" && !strings.HasPrefix(text, "/") {
		searchMuseums(ctx, api, pool, chatId, userId, text)
	}
}

func handleCallback(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, upd *schemes.MessageCallbackUpdate) {
	chatId := upd.GetChatID()
	userId := upd.GetUserID()
	payload := upd.Callback.Payload
	cbId := upd.Callback.CallbackID
	parts := strings.Split(payload, ":")

	if strings.HasPrefix(payload, "adm:") {
		session := getSession(userId)
		if session == nil {
			answerCb(ctx, api, chatId, cbId, "🚫 Сессия истекла. Войдите заново.", nil)
			return
		}
		role := session.Role
		action := parts[1]
		switch action {
		case "admin_menu":
			showAdminMenu(ctx, api, chatId, role, cbId)
		case "logout":
			clearSession(userId)
			clearUserState(userId)
			sendMainMenu(ctx, api, chatId, cbId)
		case "add_mus":
			if role != RoleBotAdmin {
				answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
				return
			}
			showAddMuseum(ctx, api, pool, chatId, userId, cbId)
		case "list_mus":
			page := 0
			if len(parts) >= 3 {
				if p, err := strconv.Atoi(parts[2]); err == nil && p >= 0 {
					page = p
				}
			}
			showMuseumManageList(ctx, api, pool, chatId, userId, role, page, cbId)
		case "manage_mus":
			if len(parts) < 3 {
				return
			}
			mid, _ := strconv.ParseInt(parts[2], 10, 64)
			showMuseumManage(ctx, api, pool, chatId, userId, role, mid, cbId)
		case "edit_mus":
			if len(parts) < 3 {
				return
			}
			mid, _ := strconv.ParseInt(parts[2], 10, 64)
			if !canAccessMuseum(ctx, pool, userId, role, mid) {
				answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
				return
			}
			setUserState(userId, "awaiting_museum_edit", parts[2])
			kb := api.Messages.NewKeyboardBuilder()
			kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
			answerCb(ctx, api, chatId, cbId, "✏️ Редактирование музея\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте:\n\nНазвание\nКраткое название\nОписание\nАдрес\nТелефон\nEmail\nСайт\nЧасы работы", kb)
		case "del_mus":
			if role != RoleBotAdmin || len(parts) < 3 {
				return
			}
			mid, _ := strconv.ParseInt(parts[2], 10, 64)
			pool.Exec(ctx, "DELETE FROM museums WHERE id=$1", mid)
			showAdminMenu(ctx, api, chatId, role, cbId)
		case "set_photo_mus":
			if len(parts) < 3 {
				return
			}
			mid, _ := strconv.ParseInt(parts[2], 10, 64)
			if !canAccessMuseum(ctx, pool, userId, role, mid) {
				answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
				return
			}
			setUserState(userId, "awaiting_museum_photo", parts[2])
			answerCb(ctx, api, chatId, cbId, "📷 Загрузка фото музея\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте одну фотографию.", photoCancelKeyboard(api))
		case "add_exh":
			page := 0
			if len(parts) >= 3 {
				if p, err := strconv.Atoi(parts[2]); err == nil && p >= 0 {
					page = p
				}
			}
			showAddExhibition(ctx, api, pool, chatId, userId, role, page, cbId)
		case "manage_exbn":
			if len(parts) < 3 {
				return
			}
			eid, _ := strconv.ParseInt(parts[2], 10, 64)
			showExhibitionManage(ctx, api, pool, chatId, userId, role, eid, cbId)
		case "list_exbn":
			if len(parts) < 3 {
				return
			}
			museumId, _ := strconv.ParseInt(parts[2], 10, 64)
			page := 0
			if len(parts) >= 4 {
				if p, err := strconv.Atoi(parts[3]); err == nil && p >= 0 {
					page = p
				}
			}
			showMuseumExhibitionsList(ctx, api, pool, chatId, userId, role, museumId, page, cbId)
		case "del_exbn":
			if len(parts) < 3 {
				return
			}
			eid, _ := strconv.ParseInt(parts[2], 10, 64)
			pool.Exec(ctx, "DELETE FROM exhibitions WHERE id=$1", eid)
			showAdminMenu(ctx, api, chatId, role, cbId)
		case "set_photo_exh":
			if len(parts) < 3 {
				return
			}
			setUserState(userId, "awaiting_exhibition_photo", parts[2])
			answerCb(ctx, api, chatId, cbId, "📷 Загрузка фото выставки\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте одну фотографию.", photoCancelKeyboard(api))
		case "add_exbt":
			if len(parts) < 3 {
				return
			}
			setUserState(userId, "awaiting_exhibit_data", parts[2])
			kb := api.Messages.NewKeyboardBuilder()
			kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
			answerCb(ctx, api, chatId, cbId, "🖼 Добавление экспоната\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте:\n\nНазвание\nОписание", kb)
		case "del_exbt":
			if len(parts) < 3 {
				return
			}
			eid, _ := strconv.ParseInt(parts[2], 10, 64)
			pool.Exec(ctx, "DELETE FROM exhibits WHERE id=$1", eid)
			if len(parts) >= 5 {
				exbnId, _ := strconv.ParseInt(parts[3], 10, 64)
				page := 0
				if p, err := strconv.Atoi(parts[4]); err == nil && p >= 0 {
					page = p
				}
				showExhibitManageList(ctx, api, pool, chatId, userId, role, exbnId, page, cbId)
			} else {
				showAdminMenu(ctx, api, chatId, role, cbId)
			}
		case "set_photo_exbt":
			if len(parts) < 3 {
				return
			}
			setUserState(userId, "awaiting_exhibit_photo", parts[2])
			answerCb(ctx, api, chatId, cbId, "📷 Загрузка фото экспоната\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте одну фотографию.", photoCancelKeyboard(api))
		case "list_exbt":
			if len(parts) < 3 {
				return
			}
			exbnId, _ := strconv.ParseInt(parts[2], 10, 64)
			page := 0
			if len(parts) >= 4 {
				if p, err := strconv.Atoi(parts[3]); err == nil && p >= 0 {
					page = p
				}
			}
			showExhibitManageList(ctx, api, pool, chatId, userId, role, exbnId, page, cbId)
		case "add_event":
			page := 0
			if len(parts) >= 3 {
				if p, err := strconv.Atoi(parts[2]); err == nil && p >= 0 {
					page = p
				}
			}
			showAddEvent(ctx, api, pool, chatId, userId, role, page, cbId)
		case "list_events":
			if len(parts) < 3 {
				return
			}
			museumId, _ := strconv.ParseInt(parts[2], 10, 64)
			page := 0
			if len(parts) >= 4 {
				if p, err := strconv.Atoi(parts[3]); err == nil && p >= 0 {
					page = p
				}
			}
			showMuseumEventsList(ctx, api, pool, chatId, userId, role, museumId, page, cbId)
		case "del_event":
			if len(parts) < 3 {
				return
			}
			eid, _ := strconv.ParseInt(parts[2], 10, 64)
			pool.Exec(ctx, "DELETE FROM events WHERE id=$1", eid)
			if len(parts) >= 5 {
				museumId, _ := strconv.ParseInt(parts[3], 10, 64)
				page := 0
				if p, err := strconv.Atoi(parts[4]); err == nil && p >= 0 {
					page = p
				}
				showMuseumEventsList(ctx, api, pool, chatId, userId, role, museumId, page, cbId)
			} else {
				showAdminMenu(ctx, api, chatId, role, cbId)
			}
		case "staff":
			showStaffManagement(ctx, api, pool, chatId, role, cbId)
		case "add_staff":
			if role != RoleBotAdmin {
				return
			}
			setUserState(userId, "awaiting_staff_data", "")
			kb := api.Messages.NewKeyboardBuilder()
			kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
			answerCb(ctx, api, chatId, cbId, "👤 Добавление сотрудника\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте:\n\nID пользователя\nEmail\nРоль (bot_admin / museum_admin / content_manager / analyst)\nИмя\nID музея (0 если нет)", kb)
		case "del_staff":
			if role != RoleBotAdmin || len(parts) < 3 {
				return
			}
			sid, _ := strconv.ParseInt(parts[2], 10, 64)
			if sid == userId {
				answerCb(ctx, api, chatId, cbId, "❌ Нельзя удалить себя!", nil)
				return
			}
			pool.Exec(ctx, "DELETE FROM staff WHERE user_id=$1", sid)
			clearSession(sid)
			showStaffManagement(ctx, api, pool, chatId, role, cbId)
		case "edit_staff":
			if role != RoleBotAdmin || len(parts) < 3 {
				return
			}
			setUserState(userId, "awaiting_staff_edit", parts[2])
			kb := api.Messages.NewKeyboardBuilder()
			kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
			answerCb(ctx, api, chatId, cbId, "✏️ Редактирование сотрудника\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте:\n\nEmail\nРоль\nИмя\nID музея (0 если нет)", kb)
		case "analytics":
			showAnalytics(ctx, api, pool, chatId, userId, role, cbId)
		case "reviews":
			showReviews(ctx, api, pool, chatId, userId, role, cbId)
		case "train_model":
			if len(parts) < 3 {
				return
			}
			mid, _ := strconv.ParseInt(parts[2], 10, 64)
			if !canAccessMuseum(ctx, pool, userId, role, mid) {
				answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
				return
			}
			answerCb(ctx, api, chatId, cbId, fmt.Sprintf(
				"🤖 Обучение модели AI\n━━━━━━━━━━━━━━━━━━━━\n\n"+
					"Музей ID: %d\n\n"+
					"Выполните в консоли:\n"+
					"cd web_app/backend\n"+
					"python -m ml.train --museum_id %d\n\n"+
					"После завершения кнопка 🔬 заработает!", mid, mid), nil)
		case "webapp_help":
			answerCb(ctx, api, chatId, cbId,
				"🔬 Интерактивный музей временно недоступен\n━━━━━━━━━━━━━━━━━━━━\n\n"+
					"Нужно задать публичный URL в переменной WEB_APP_URL (например, ngrok/production URL).\n"+
					"URL вида localhost/127.0.0.1 не подходит для кнопок в Max.", nil)
		default:
			answerCb(ctx, api, chatId, cbId, "🔧 Функция в разработке.", nil)
		}
		return
	}

	if parts[0] == "main" {
		clearUserState(userId)
		sendMainMenu(ctx, api, chatId, cbId)
		return
	}
	if len(parts) < 2 {
		return
	}
	id, _ := strconv.ParseInt(parts[1], 10, 64)
	switch parts[0] {
	case "mus_list":
		showMuseums(ctx, api, pool, chatId, int(id), cbId)
	case "view_mus":
		showMuseumDetails(ctx, api, pool, chatId, id, userId)
	case "view_exbn":
		showExhibitionDetails(ctx, api, pool, chatId, id, userId)
	case "view_exbt":
		showExhibitDetails(ctx, api, pool, chatId, id)
	case "view_event":
		showEventDetails(ctx, api, pool, chatId, id, cbId)
	case "register_event":
		registerForEvent(ctx, api, pool, chatId, userId, id, cbId)
	case "rate_menu":
		showRateMenu(ctx, api, pool, chatId, userId, id, cbId)
	case "rate_museum":
		if len(parts) >= 3 {
			handleRate(ctx, api, pool, chatId, userId, id, parts[2], cbId)
		}
	case "write_review":
		setUserState(userId, "awaiting_review_text", fmt.Sprintf("%d", id))
		kb := api.Messages.NewKeyboardBuilder()
		kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "main")
		answerCb(ctx, api, chatId, cbId, "💬 Напишите отзыв о музее\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте текст", kb)
	case "add_exh_mus":
		setUserState(userId, "awaiting_exhibition_data", fmt.Sprintf("%d", id))
		kb := api.Messages.NewKeyboardBuilder()
		kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
		answerCb(ctx, api, chatId, cbId, "🎨 Добавление выставки\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте:\n\nНазвание\nОписание", kb)
	case "add_event_mus":
		setUserState(userId, "awaiting_event_data", fmt.Sprintf("%d", id))
		kb := api.Messages.NewKeyboardBuilder()
		kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
		answerCb(ctx, api, chatId, cbId, "📅 Добавление мероприятия\n━━━━━━━━━━━━━━━━━━━━\n\nОтправьте:\n\nНазвание\nОписание\nДата (ДД.ММ.ГГГГ ЧЧ:ММ)\nМакс. участников\nЦена (0 — бесплатно)", kb)
	}
}

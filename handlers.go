package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"net/smtp"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

// ==========================================
// ЗАГРУЗКА ФОТО
// ==========================================

func handlePhotoUpload(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, state *UserState, photo *schemes.PhotoAttachment) {
	token := photo.Payload.Token
	if token == "" && photo.Payload.Url != "" {
		info, err := api.Uploads.UploadMediaFromUrl(ctx, schemes.PHOTO, photo.Payload.Url)
		if err != nil {
			_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Не удалось загрузить фото. Попробуйте ещё раз."))
			clearUserState(userId)
			return
		}
		token = info.Token
	}
	if token == "" {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Не удалось обработать фотографию."))
		clearUserState(userId)
		return
	}
	// Prefer the CDN URL (photo.Payload.Url) for web display.
	// Fall back to the attachment token only when no URL is available.
	imageRef := photo.Payload.Url
	if imageRef == "" {
		imageRef = token
	}
	entityId, _ := strconv.ParseInt(state.ExtraData, 10, 64)
	var table string
	switch state.State {
	case "awaiting_museum_photo":
		table = "museums"
	case "awaiting_exhibition_photo":
		table = "exhibitions"
	case "awaiting_exhibit_photo":
		table = "exhibits"
	}
	_, err := pool.Exec(ctx, fmt.Sprintf("UPDATE %s SET image_url=$1 WHERE id=$2", table), imageRef, entityId)
	clearUserState(userId)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка сохранения фото. Попробуйте позже."))
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.POSITIVE, "adm:admin_menu")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText("✅ Фото успешно обновлено!"))
}

func photoCancelKeyboard(api *maxbot.Api) *maxbot.Keyboard {
	return adminCancelKeyboard(api)
}

func adminCancelKeyboard(api *maxbot.Api) *maxbot.Keyboard {
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
	return kb
}

func mainCancelKeyboard(api *maxbot.Api) *maxbot.Keyboard {
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "main")
	return kb
}

// ==========================================
// ОБРАБОТЧИКИ ВВОДА
// ==========================================

func handleAddMuseumData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text string) {
	lines := strings.Split(text, "\n")
	if len(lines) < 4 {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Минимум 4 строки:\nНазвание\nКраткое название\nОписание\nАдрес\n\nИли нажмите «❌ Отмена»."))
		return
	}
	name, short := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	desc, addr := strings.TrimSpace(lines[2]), strings.TrimSpace(lines[3])
	var newId int64
	err := pool.QueryRow(ctx,
		"INSERT INTO museums (name,short_name,description,address,phone,email,website,opening_hours) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id",
		name, short, desc, addr, optLine(lines, 4), optLine(lines, 5), optLine(lines, 6), optLine(lines, 7)).Scan(&newId)
	clearUserState(userId)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка добавления музея."))
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("📷 Добавить фото", schemes.POSITIVE, fmt.Sprintf("adm:set_photo_mus:%d", newId))
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(fmt.Sprintf("✅ Музей «%s» добавлен!", name)))
}

func handleEditMuseumData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text, midStr string) {
	mid, _ := strconv.ParseInt(midStr, 10, 64)
	lines := strings.Split(text, "\n")
	if len(lines) < 4 {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Минимум 4 строки. Или нажмите «❌ Отмена»."))
		return
	}
	name, short := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	desc, addr := strings.TrimSpace(lines[2]), strings.TrimSpace(lines[3])
	_, err := pool.Exec(ctx,
		"UPDATE museums SET name=$1,short_name=$2,description=$3,address=$4,phone=$5,email=$6,website=$7,opening_hours=$8 WHERE id=$9",
		name, short, desc, addr, optLine(lines, 4), optLine(lines, 5), optLine(lines, 6), optLine(lines, 7), mid)
	clearUserState(userId)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка обновления."))
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText("✅ Музей обновлён!"))
}

func handleAddExhibitionData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text, museumIdStr string) {
	museumId, _ := strconv.ParseInt(museumIdStr, 10, 64)
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Минимум 2 строки: Название и Описание. Или нажмите «❌ Отмена»."))
		return
	}
	title, desc := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	var newId int64
	err := pool.QueryRow(ctx, "INSERT INTO exhibitions (museum_id,title,description) VALUES ($1,$2,$3) RETURNING id",
		museumId, title, desc).Scan(&newId)
	clearUserState(userId)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка добавления выставки."))
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("📷 Добавить фото", schemes.POSITIVE, fmt.Sprintf("adm:set_photo_exh:%d", newId))
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(fmt.Sprintf("✅ Выставка «%s» добавлена!", title)))
}

func handleAddExhibitData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text, exbnIdStr string) {
	exbnId, _ := strconv.ParseInt(exbnIdStr, 10, 64)
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Минимум 2 строки: Название и Описание. Или нажмите «❌ Отмена»."))
		return
	}
	title, desc := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	var newId int64
	err := pool.QueryRow(ctx, "INSERT INTO exhibits (exhibition_id,title,description) VALUES ($1,$2,$3) RETURNING id",
		exbnId, title, desc).Scan(&newId)
	clearUserState(userId)
	if err != nil {
		log.Printf("Ошибка добавления экспоната (exhibition_id=%d): %v", exbnId, err)
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка добавления экспоната."))
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("📷 Добавить фото", schemes.POSITIVE, fmt.Sprintf("adm:set_photo_exbt:%d", newId))
	kb.AddRow().AddCallback("🎨 К выставке", schemes.DEFAULT, fmt.Sprintf("adm:manage_exbn:%d", exbnId))
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(fmt.Sprintf("✅ Экспонат «%s» добавлен!", title)))
}

func handleAddEventData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text, museumIdStr string) {
	museumId, _ := strconv.ParseInt(museumIdStr, 10, 64)
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Минимум 3 строки. Или нажмите «❌ Отмена»."))
		return
	}
	title, desc := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	eventDate, err := time.Parse("02.01.2006 15:04", strings.TrimSpace(lines[2]))
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Формат даты: ДД.ММ.ГГГГ ЧЧ:ММ"))
		return
	}
	maxPart := 0
	price := 0.0
	if len(lines) > 3 {
		maxPart, _ = strconv.Atoi(strings.TrimSpace(lines[3]))
	}
	if len(lines) > 4 {
		price, _ = strconv.ParseFloat(strings.TrimSpace(lines[4]), 64)
	}
	pool.Exec(ctx, "INSERT INTO events (museum_id,title,description,event_date,max_participants,price) VALUES ($1,$2,$3,$4,$5,$6)",
		museumId, title, desc, eventDate, maxPart, price)
	clearUserState(userId)
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(
		fmt.Sprintf("✅ Мероприятие «%s» добавлено!\n📅 %s", title, eventDate.Format("02.01.2006 15:04"))))
}

func handleAddStaffData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text string) {
	lines := strings.Split(text, "\n")
	if len(lines) < 4 {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Минимум 4 строки. Или нажмите «❌ Отмена»."))
		return
	}
	staffUid, err := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ ID должен быть числом."))
		return
	}
	email, role, alias := strings.TrimSpace(lines[1]), strings.TrimSpace(lines[2]), strings.TrimSpace(lines[3])
	if !isValidRole(role) {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Роли: bot_admin, museum_admin, content_manager, analyst"))
		return
	}
	var museumId *int64
	if len(lines) > 4 {
		if mid, _ := strconv.ParseInt(strings.TrimSpace(lines[4]), 10, 64); mid > 0 {
			museumId = &mid
		}
	}
	_, err = pool.Exec(ctx, "INSERT INTO staff (user_id,email,role,alias,museum_id) VALUES ($1,$2,$3,$4,$5)", staffUid, email, role, alias, museumId)
	clearUserState(userId)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка. Возможно, сотрудник уже существует."))
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("👥 Персонал", schemes.POSITIVE, "adm:staff")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(
		fmt.Sprintf("✅ Сотрудник «%s» добавлен!\n🆔 %d | 📧 %s | 🔑 %s", alias, staffUid, email, role)))
}

func handleEditStaffData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text, sidStr string) {
	staffUid, _ := strconv.ParseInt(sidStr, 10, 64)
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Минимум 3 строки. Или нажмите «❌ Отмена»."))
		return
	}
	email, role, alias := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1]), strings.TrimSpace(lines[2])
	if !isValidRole(role) {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Неверная роль!"))
		return
	}
	var museumId *int64
	if len(lines) > 3 {
		if mid, _ := strconv.ParseInt(strings.TrimSpace(lines[3]), 10, 64); mid > 0 {
			museumId = &mid
		}
	}
	pool.Exec(ctx, "UPDATE staff SET email=$1,role=$2,alias=$3,museum_id=$4 WHERE user_id=$5", email, role, alias, museumId, staffUid)
	clearUserState(userId)
	clearSession(staffUid)
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("👥 Персонал", schemes.POSITIVE, "adm:staff")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText("✅ Сотрудник обновлён!"))
}

func handleReviewText(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text, midStr string) {
	museumId, _ := strconv.ParseInt(midStr, 10, 64)
	comment := strings.TrimSpace(text)
	if len(comment) > 1000 {
		comment = comment[:1000]
	}
	pool.Exec(ctx, "UPDATE reviews SET comment=$1 WHERE museum_id=$2 AND user_id=$3", comment, museumId, userId)
	clearUserState(userId)
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("🏛 К музею", schemes.POSITIVE, fmt.Sprintf("view_mus:%d", museumId))
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText("✅ Отзыв сохранён! Спасибо!"))
}

// ==========================================
// ОЦЕНКА МУЗЕЯ
// ==========================================

func showRateMenu(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId, museumId int64, cbId string) {
	var existingRating int
	var existingComment *string
	err := pool.QueryRow(ctx, "SELECT rating, comment FROM reviews WHERE museum_id=$1 AND user_id=$2", museumId, userId).
		Scan(&existingRating, &existingComment)
	kb := api.Messages.NewKeyboardBuilder()
	text := "⭐ Оцените музей\n━━━━━━━━━━━━━━━━━━━━\n\n"
	if err == nil {
		text += fmt.Sprintf("Ваша оценка: %s%s\n", strings.Repeat("⭐", existingRating), strings.Repeat("☆", 5-existingRating))
		if existingComment != nil && *existingComment != "" {
			text += fmt.Sprintf("Ваш отзыв: %s\n", *existingComment)
		}
		text += "\nИзменить оценку:"
	} else {
		text += "Выберите оценку от 1 до 5:"
	}
	for i := 1; i <= 5; i++ {
		kb.AddRow().AddCallback(fmt.Sprintf("%d %s", i, strings.Repeat("⭐", i)), schemes.DEFAULT, fmt.Sprintf("rate_museum:%d:%d", museumId, i))
	}
	kb.AddRow().AddCallback("↩️ Назад к музею", schemes.NEGATIVE, fmt.Sprintf("view_mus:%d", museumId))
	answerCb(ctx, api, chatId, cbId, text, kb)
}

func handleRate(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId, museumId int64, ratingStr, cbId string) {
	rating, err := strconv.Atoi(ratingStr)
	if err != nil || rating < 1 || rating > 5 {
		return
	}
	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM reviews WHERE museum_id=$1 AND user_id=$2", museumId, userId).Scan(&count)
	if count > 0 {
		pool.Exec(ctx, "UPDATE reviews SET rating=$1 WHERE museum_id=$2 AND user_id=$3", rating, museumId, userId)
	} else {
		pool.Exec(ctx, "INSERT INTO reviews (museum_id,user_id,rating) VALUES ($1,$2,$3)", museumId, userId, rating)
	}
	filled := strings.Repeat("⭐", rating)
	empty := strings.Repeat("☆", 5-rating)
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("💬 Написать отзыв", schemes.POSITIVE, fmt.Sprintf("write_review:%d", museumId))
	kb.AddRow().AddCallback("🏛 К музею", schemes.DEFAULT, fmt.Sprintf("view_mus:%d", museumId))
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	msg := "🌟 Оценка сохранена!\n━━━━━━━━━━━━━━━━━━━━\n\n"
	if count > 0 {
		msg += "Обновлена: "
	} else {
		msg += "Ваша оценка: "
	}
	msg += filled + empty + "\n\nХотите оставить отзыв?"
	answerCb(ctx, api, chatId, cbId, msg, kb)
}

// ==========================================
// 2FA
// ==========================================

func startAuthProcess(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64) {
	session := getSession(userId)
	if session != nil {
		showAdminMenu(ctx, api, chatId, session.Role, "")
		return
	}
	var email string
	err := pool.QueryRow(ctx, "SELECT email FROM staff WHERE user_id=$1 AND is_active=true", userId).Scan(&email)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("🚫 Доступ запрещён\n━━━━━━━━━━━━━━━━━━━━\n\nВаш ID не найден в списке сотрудников."))
		return
	}
	code := rand.Intn(900000) + 100000
	if _, err = pool.Exec(ctx, "UPDATE staff SET current_otp=$1, otp_expires_at=$2 WHERE user_id=$3",
		fmt.Sprintf("%d", code), time.Now().Add(5*time.Minute), userId); err != nil {
		log.Printf("OTP update error for user %d: %v", userId, err)
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Не удалось создать код входа. Попробуйте позже."))
		return
	}
	if err = sendAuthEmail(email, code); err != nil {
		log.Printf("OTP send error for user %d (%s): %v", userId, email, err)
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText(
			"❌ Не удалось отправить код на почту.\nПроверьте SMTP-настройки и попробуйте снова.\n\n"))
		return
	}
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText(
		fmt.Sprintf("🔐 Авторизация сотрудника\n━━━━━━━━━━━━━━━━━━━━\n\n📩 Код отправлен: %s\n⏱ Действует 5 минут\n\nВведите 6 цифр одним сообщением", maskEmail(email))))
}

func verify2FA(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, code int) {
	var dbCode *string
	var expiry *time.Time
	var role string
	err := pool.QueryRow(ctx, "SELECT current_otp, otp_expires_at, role FROM staff WHERE user_id=$1", userId).
		Scan(&dbCode, &expiry, &role)
	if err != nil || dbCode == nil || *dbCode != fmt.Sprintf("%d", code) || time.Now().After(*expiry) {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("⚠️ Неверный код или срок действия истёк.\nПопробуйте запросить код ещё раз."))
		return
	}
	pool.Exec(ctx, "UPDATE staff SET current_otp=NULL, last_login=NOW() WHERE user_id=$1", userId)
	setSession(userId, role)
	showAdminMenu(ctx, api, chatId, role, "")
}

func sendAuthEmail(to string, code int) error {
	from := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	if host == "" {
		host = "smtp.gmail.com"
	}
	if port == "" {
		port = "587"
	}
	if from == "" || pass == "" {
		return fmt.Errorf("SMTP не настроен: SMTP_USER/SMTP_PASS")
	}
	subject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte("Код входа — Музеи СК")) + "?="
	body := fmt.Sprintf("━━━━━━━━━━━━━━━━━━━━\n  МУЗЕИ СТАВРОПОЛЬСКОГО КРАЯ\n━━━━━━━━━━━━━━━━━━━━\n\nВаш код: %06d\n\nКод действует 5 минут.\nНикому не сообщайте.\n\n━━━━━━━━━━━━━━━━━━━━\n", code)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, to, subject, body)
	auth := smtp.PlainAuth("", from, pass, host)
	if err := smtp.SendMail(host+":"+port, auth, from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("SMTP send error: %w", err)
	}
	log.Printf("2FA -> %s", to)
	return nil
}

// ==========================================
// МЕНЮ И НАВИГАЦИЯ
// ==========================================

func showAdminMenu(ctx context.Context, api *maxbot.Api, chatId int64, role, cbId string) {
	kb := api.Messages.NewKeyboardBuilder()
	names := map[string]string{RoleBotAdmin: "Главный администратор", RoleMuseumAdmin: "Администратор музея", RoleContentManager: "Контент-менеджер", RoleAnalyst: "Аналитик"}
	text := "🛡 Панель управления\n━━━━━━━━━━━━━━━━━━━━\n\n👤 " + names[role] + "\n\nВыберите действие:"
	switch role {
	case RoleBotAdmin:
		text += "\n\n➕ Музей • 🏛 Музеи • 🎨 Выставка • 📅 Событие • 👥 Команда"
		kb.AddRow().AddCallback("➕ Музей", schemes.POSITIVE, "adm:add_mus").
			AddCallback("🏛 Музеи", schemes.DEFAULT, "adm:list_mus")
		kb.AddRow().AddCallback("🎨 Выставка", schemes.POSITIVE, "adm:add_exh").
			AddCallback("📅 Событие", schemes.POSITIVE, "adm:add_event")
		kb.AddRow().AddCallback("👥 Команда", schemes.DEFAULT, "adm:staff")
		kb.AddRow().AddCallback("📊 Аналитика", schemes.DEFAULT, "adm:analytics").
			AddCallback("💬 Отзывы", schemes.DEFAULT, "adm:reviews")
	case RoleMuseumAdmin:
		text += "\n\n🏛 Музеи • 🎨 Выставка • 📅 Событие"
		kb.AddRow().AddCallback("🏛 Музеи", schemes.DEFAULT, "adm:list_mus")
		kb.AddRow().AddCallback("🎨 Выставка", schemes.POSITIVE, "adm:add_exh").
			AddCallback("📅 Событие", schemes.POSITIVE, "adm:add_event")
		kb.AddRow().AddCallback("📊 Аналитика", schemes.DEFAULT, "adm:analytics").
			AddCallback("💬 Отзывы", schemes.DEFAULT, "adm:reviews")
	case RoleContentManager:
		text += "\n\n🏛 Музеи • 🎨 Выставка • 📅 Событие"
		kb.AddRow().AddCallback("🏛 Музеи", schemes.DEFAULT, "adm:list_mus")
		kb.AddRow().AddCallback("🎨 Выставка", schemes.POSITIVE, "adm:add_exh").
			AddCallback("📅 Событие", schemes.POSITIVE, "adm:add_event")
	case RoleAnalyst:
		kb.AddRow().AddCallback("📊 Аналитика", schemes.DEFAULT, "adm:analytics").
			AddCallback("💬 Отзывы", schemes.DEFAULT, "adm:reviews")
	}
	kb.AddRow().AddCallback("🚪 Выйти", schemes.NEGATIVE, "adm:logout").
		AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	answerCb(ctx, api, chatId, cbId, text, kb)
}

func sendMainMenu(ctx context.Context, api *maxbot.Api, chatId int64, cbId string) {
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddMessage("🏛 Список музеев")
	kb.AddRow().AddMessage("📍 Ближайшие")
	kb.AddRow().AddMessage("🔍 Поиск музеев")
	kb.AddRow().AddMessage("📅 Мероприятия")
	if webURL, ok := getPublicWebAppURL(); ok {
		kb.AddRow().AddOpenApp("🔬 Мини-апп", webURL, "", 0)
	}
	if browserURL, ok := getBrowserWebAppURL(); ok {
		kb.AddRow().AddLink("🌐 Открыть в браузере", schemes.DEFAULT, browserURL)
	}
	kb.AddRow().AddMessage("👨‍💻 Вход для сотрудников")
	answerCb(ctx, api, chatId, cbId, "🏛 Музеи Ставропольского края\n━━━━━━━━━━━━━━━━━━━━\n\nДобро пожаловать! Выберите действие:", kb)
}

func showMuseums(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId int64, page int, cbId string) {
	offset := page * 10
	rows, err := pool.Query(ctx, "SELECT id, COALESCE(NULLIF(short_name,''), name) FROM museums ORDER BY name LIMIT 11 OFFSET $1", offset)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Ошибка загрузки.", nil)
		return
	}
	defer rows.Close()
	kb := api.Messages.NewKeyboardBuilder()
	count := 0
	for rows.Next() {
		if count < 10 {
			var id int64
			var name string
			if rows.Scan(&id, &name) == nil {
				kb.AddRow().AddCallback(name, schemes.POSITIVE, fmt.Sprintf("view_mus:%d", id))
			}
		}
		count++
	}
	if page > 0 {
		kb.AddRow().AddCallback("⬅️ Назад", schemes.NEGATIVE, fmt.Sprintf("mus_list:%d", page-1))
	}
	if count > 10 {
		kb.AddRow().AddCallback("Вперёд ➡️", schemes.NEGATIVE, fmt.Sprintf("mus_list:%d", page+1))
	}
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.POSITIVE, "main")
	answerCb(ctx, api, chatId, cbId, "🏛 Музеи Ставропольского края\n━━━━━━━━━━━━━━━━━━━━\n\nВыберите музей:", kb)
}

func showMuseumDetails(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, id, userId int64) {
	var m struct {
		name                   string
		desc, addr, hours      *string
		phone, email, web, img *string
		lat, lon               *float64
	}
	err := pool.QueryRow(ctx,
		"SELECT name,description,address,opening_hours,phone,email,website,image_url,latitude,longitude FROM museums WHERE id=$1", id).
		Scan(&m.name, &m.desc, &m.addr, &m.hours, &m.phone, &m.email, &m.web, &m.img, &m.lat, &m.lon)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("⚠️ Музей не найден."))
		return
	}
	text := fmt.Sprintf("🏛 %s\n━━━━━━━━━━━━━━━━━━━━\n\n", m.name)
	if m.desc != nil {
		text += *m.desc + "\n"
	}
	if m.addr != nil {
		text += fmt.Sprintf("\n📍 %s", *m.addr)
	}
	if m.hours != nil {
		text += fmt.Sprintf("\n⏰ %s", *m.hours)
	}
	if m.phone != nil {
		text += fmt.Sprintf("\n📞 %s", *m.phone)
	}
	if m.email != nil {
		text += fmt.Sprintf("\n📧 %s", *m.email)
	}
	if m.web != nil {
		text += fmt.Sprintf("\n🌐 %s", *m.web)
	}
	var avgRating *float64
	var reviewCount int
	pool.QueryRow(ctx, "SELECT AVG(rating)::numeric(3,1), COUNT(*) FROM reviews WHERE museum_id=$1", id).Scan(&avgRating, &reviewCount)
	if avgRating != nil && reviewCount > 0 {
		stars := int(*avgRating + 0.5)
		if stars > 5 {
			stars = 5
		}
		text += fmt.Sprintf("\n\n%s%s  %.1f / 5  (%d отз.)", strings.Repeat("⭐", stars), strings.Repeat("☆", 5-stars), *avgRating, reviewCount)
	}
	kb := api.Messages.NewKeyboardBuilder()
	exRows, err := pool.Query(ctx, "SELECT id, title FROM exhibitions WHERE museum_id=$1 AND is_active=true ORDER BY title LIMIT 10", id)
	if err == nil {
		first := true
		for exRows.Next() {
			if first {
				text += "\n\n🎨 ВЫСТАВКИ:"
				first = false
			}
			var eid int64
			var title string
			if exRows.Scan(&eid, &title) == nil {
				kb.AddRow().AddCallback("🔹 "+title, schemes.POSITIVE, fmt.Sprintf("view_exbn:%d", eid))
			}
		}
		exRows.Close()
	}
	kb.AddRow().AddCallback("⭐ Оценить музей", schemes.DEFAULT, fmt.Sprintf("rate_menu:%d", id))
	if webUrl, ok := getMuseumWebAppURL(id); ok {
		kb.AddRow().AddOpenApp("🔬 Мини-апп (AI)", webUrl, strconv.FormatInt(id, 10), 0)
	}
	if browserURL, ok := getMuseumWebBrowserURL(id); ok {
		kb.AddRow().AddLink("🌐 Открыть в браузере", schemes.DEFAULT, browserURL)
	}
	if m.addr != nil {
		mapUrl := fmt.Sprintf("https://yandex.ru/maps/?text=%s", url.QueryEscape(*m.addr))
		kb.AddRow().AddLink("🗺 Построить маршрут", schemes.POSITIVE, mapUrl)
	}
	session := getSession(userId)
	if session != nil {
		r := session.Role
		if canAccessMuseum(ctx, pool, userId, r, id) && (r == RoleBotAdmin || r == RoleMuseumAdmin) {
			kb.AddRow().AddCallback("✏️ Управление музеем", schemes.DEFAULT, fmt.Sprintf("adm:manage_mus:%d", id))
		}
	}
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.POSITIVE, "main")
	msg := maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text)
	hasPhoto := false
	if m.img != nil && *m.img != "" {
		hasPhoto = attachPhoto(ctx, api, msg, *m.img)
	}
	if err := api.Messages.Send(ctx, msg); err != nil {
		log.Printf("showMuseumDetails send error: %v", err)
		if hasPhoto {
			fallback := maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text)
			if retryErr := api.Messages.Send(ctx, fallback); retryErr != nil {
				log.Printf("showMuseumDetails fallback send error: %v", retryErr)
			}
		}
	}
}

// ==========================================
// ВСПОМОГАТЕЛЬНЫЕ
// ==========================================

func maskEmail(e string) string {
	parts := strings.Split(e, "@")
	if len(parts) != 2 {
		return e
	}
	if len(parts[0]) < 3 {
		return "***@" + parts[1]
	}
	return parts[0][:2] + "***@" + parts[1]
}

func optLine(lines []string, idx int) *string {
	if idx < len(lines) {
		v := strings.TrimSpace(lines[idx])
		if v != "" {
			return &v
		}
	}
	return nil
}

func isValidRole(r string) bool {
	return r == "bot_admin" || r == "museum_admin" || r == "content_manager" || r == "analyst"
}

func notifyEventRegistrants(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, eventId int64, message string) {
	rows, err := pool.Query(ctx, "SELECT chat_id FROM event_registrations WHERE event_id=$1", eventId)
	if err != nil {
		log.Printf("notifyEventRegistrants query error: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var chatId int64
		if rows.Scan(&chatId) == nil {
			if err := api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText(message)); err != nil {
				log.Printf("notifyEventRegistrants send to chat %d error: %v", chatId, err)
			}
		}
	}
}

func handleEditEventDateData(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, text, eventIdStr string) {
	eventId, _ := strconv.ParseInt(eventIdStr, 10, 64)
	newDate, err := time.Parse("02.01.2006 15:04", strings.TrimSpace(text))
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(adminCancelKeyboard(api)).SetText("❌ Формат даты: ДД.ММ.ГГГГ ЧЧ:ММ"))
		return
	}
	var title string
	var oldDate time.Time
	err = pool.QueryRow(ctx, "SELECT title, event_date FROM events WHERE id=$1", eventId).Scan(&title, &oldDate)
	if err != nil {
		clearUserState(userId)
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Мероприятие не найдено."))
		return
	}
	pool.Exec(ctx, "UPDATE events SET event_date=$1 WHERE id=$2", newDate, eventId)
	pool.Exec(ctx, "UPDATE event_registrations SET reminded=false WHERE event_id=$1", eventId)
	clearUserState(userId)
	go notifyEventRegistrants(ctx, api, pool, eventId, fmt.Sprintf(
		"⚠️ Изменение времени!\n━━━━━━━━━━━━━━━━━━━━\n\n🎭 %s\n\nБыло: %s\nСтало: %s",
		title, oldDate.Format("02.01.2006 15:04"), newDate.Format("02.01.2006 15:04")))
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("⚙️ К мероприятию", schemes.DEFAULT, fmt.Sprintf("adm:manage_event:%d", eventId))
	kb.AddRow().AddCallback("🛡 Панель", schemes.DEFAULT, "adm:admin_menu")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(
		fmt.Sprintf("✅ Дата мероприятия «%s» изменена!\n📅 %s\n\nВсе записавшиеся уведомлены.",
			title, newDate.Format("02.01.2006 15:04"))))
}

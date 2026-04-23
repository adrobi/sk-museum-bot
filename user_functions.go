package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

type MuseumDistance struct {
	ID        int64
	Name      string
	ShortName string
	Address   string
	Distance  float64
	Latitude  float64
	Longitude float64
}

func handleLocation(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId int64, lat, lon float64) {
	rows, err := pool.Query(ctx,
		"SELECT id, name, COALESCE(short_name,name), COALESCE(address,''), latitude, longitude FROM museums WHERE latitude IS NOT NULL AND longitude IS NOT NULL")
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка."))
		return
	}
	defer rows.Close()
	var museums []MuseumDistance
	for rows.Next() {
		var m MuseumDistance
		if rows.Scan(&m.ID, &m.Name, &m.ShortName, &m.Address, &m.Latitude, &m.Longitude) == nil {
			m.Distance = calculateDistance(lat, lon, m.Latitude, m.Longitude)
			museums = append(museums, m)
		}
	}
	sort.Slice(museums, func(i, j int) bool { return museums[i].Distance < museums[j].Distance })
	kb := api.Messages.NewKeyboardBuilder()
	text := "📍 Ближайшие музеи\n━━━━━━━━━━━━━━━━━━━━\n\n"
	count := 0
	for _, m := range museums {
		if count >= 3 {
			break
		}
		text += fmt.Sprintf("%d. %s\n   📍 %s\n   📏 %.1f км\n\n", count+1, m.ShortName, m.Address, m.Distance)
		kb.AddRow().AddCallback(fmt.Sprintf("%d. %s (%.1f км)", count+1, m.ShortName, m.Distance),
			schemes.POSITIVE, fmt.Sprintf("view_mus:%d", m.ID))
		count++
	}
	if count == 0 {
		text = "😔 Музеев с координатами не найдено."
	}
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text))
}

func searchMuseums(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, query string) {
	pool.Exec(ctx, "INSERT INTO search_logs (user_id, query, results_count) VALUES ($1,$2,0)", userId, query)
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := pool.Query(ctx, `
		SELECT id, name, COALESCE(NULLIF(short_name,''), name), COALESCE(address,'')
		FROM museums WHERE LOWER(name) LIKE $1 OR LOWER(COALESCE(short_name,'')) LIKE $1 OR LOWER(COALESCE(description,'')) LIKE $1
		ORDER BY name LIMIT 10`, pattern)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка поиска."))
		return
	}
	defer rows.Close()
	type R struct {
		ID   int64
		Name string
		Addr string
	}
	var results []R
	for rows.Next() {
		var r R
		var fullName string
		if rows.Scan(&r.ID, &fullName, &r.Name, &r.Addr) == nil {
			results = append(results, r)
		}
	}
	kb := api.Messages.NewKeyboardBuilder()
	if len(results) == 0 {
		kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(
			fmt.Sprintf("🔍 По запросу «%s» ничего не найдено.", query)))
		return
	}
	text := fmt.Sprintf("🔍 Результаты: «%s»\n━━━━━━━━━━━━━━━━━━━━\n\nНайдено: %d\n\n", query, len(results))
	for i, r := range results {
		text += fmt.Sprintf("%d. %s\n", i+1, r.Name)
		if r.Addr != "" {
			text += fmt.Sprintf("   📍 %s\n", r.Addr)
		}
		text += "\n"
		kb.AddRow().AddCallback(r.Name, schemes.POSITIVE, fmt.Sprintf("view_mus:%d", r.ID))
	}
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text))
}

func showEvents(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId int64) {
	rows, err := pool.Query(ctx, `
		SELECT e.id, e.title, e.event_date, e.price, COALESCE(m.short_name, m.name)
		FROM events e JOIN museums m ON e.museum_id=m.id
		WHERE e.is_active=true AND e.event_date::date >= CURRENT_DATE ORDER BY e.event_date LIMIT 10`)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Ошибка."))
		return
	}
	defer rows.Close()
	kb := api.Messages.NewKeyboardBuilder()
	text := "📅 Мероприятия\n━━━━━━━━━━━━━━━━━━━━\n\n"
	count := 0
	for rows.Next() {
		var id int64
		var title, museum string
		var eventDate time.Time
		var price float64
		if rows.Scan(&id, &title, &eventDate, &price, &museum) == nil {
			text += fmt.Sprintf("🎭 %s\n   🏛 %s\n   📅 %s\n", title, museum, eventDate.Format("02.01.2006 15:04"))
			if price > 0 {
				text += fmt.Sprintf("   💰 %.0f руб.\n\n", price)
			} else {
				text += "   💰 Бесплатно\n\n"
			}
			kb.AddRow().AddCallback("📋 "+title, schemes.POSITIVE, fmt.Sprintf("view_event:%d", id))
			count++
		}
	}
	if count == 0 {
		text = "📅 Мероприятий пока нет."
	}
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text))
}

func showEventDetails(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId, eventId int64, cbId string) {
	var title, desc, museum string
	var eventDate time.Time
	var duration, maxPart, curPart int
	var price float64
	err := pool.QueryRow(ctx, `
		SELECT e.title, COALESCE(e.description,''), e.event_date, e.duration_hours,
		       e.max_participants, e.current_participants, e.price, COALESCE(m.short_name,m.name)
		FROM events e JOIN museums m ON e.museum_id=m.id
		WHERE e.id=$1 AND e.is_active=true AND e.event_date::date >= CURRENT_DATE`, eventId).
		Scan(&title, &desc, &eventDate, &duration, &maxPart, &curPart, &price, &museum)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Мероприятие не найдено.", nil)
		return
	}
	text := fmt.Sprintf("🎭 %s\n━━━━━━━━━━━━━━━━━━━━\n\n🏛 %s\n📅 %s\n⏱ %d ч.\n",
		title, museum, eventDate.Format("02.01.2006 в 15:04"), duration)
	if price > 0 {
		text += fmt.Sprintf("💰 %.0f руб.\n", price)
	} else {
		text += "💰 Бесплатно\n"
	}
	if maxPart > 0 {
		text += fmt.Sprintf("👥 Места: %d / %d\n", curPart, maxPart)
	}
	if desc != "" {
		text += fmt.Sprintf("\n📝 %s\n", desc)
	}
	var regExists int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM event_registrations WHERE event_id=$1 AND user_id=$2", eventId, userId).Scan(&regExists)
	kb := api.Messages.NewKeyboardBuilder()
	if regExists > 0 {
		text += "\n✅ Вы записаны на это мероприятие"
		kb.AddRow().AddCallback("❌ Отменить запись", schemes.NEGATIVE, fmt.Sprintf("unreg_event:%d", eventId))
	} else if maxPart == 0 || curPart < maxPart {
		kb.AddRow().AddCallback("✅ Записаться", schemes.POSITIVE, fmt.Sprintf("register_event:%d", eventId))
	} else {
		text += "\n❌ Мест нет"
	}
	kb.AddRow().AddCallback("📅 Все мероприятия", schemes.DEFAULT, "events_list:0")
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	answerCb(ctx, api, chatId, cbId, text, kb)
}

func registerForEvent(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId, eventId int64, cbId string) {
	var eventExists int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM events WHERE id=$1 AND is_active=true AND event_date::date >= CURRENT_DATE", eventId).Scan(&eventExists)
	if eventExists == 0 {
		answerCb(ctx, api, chatId, cbId, "❌ Мероприятие недоступно.", nil)
		return
	}
	var regExists int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM event_registrations WHERE event_id=$1 AND user_id=$2", eventId, userId).Scan(&regExists)
	if regExists > 0 {
		answerCb(ctx, api, chatId, cbId, "ℹ️ Вы уже записаны на это мероприятие.", nil)
		return
	}
	var maxPart, curPart int
	pool.QueryRow(ctx, "SELECT max_participants, current_participants FROM events WHERE id=$1", eventId).Scan(&maxPart, &curPart)
	if maxPart > 0 && curPart >= maxPart {
		answerCb(ctx, api, chatId, cbId, "❌ Мест нет.", nil)
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("⏰ За 4 часа", schemes.DEFAULT, fmt.Sprintf("reg_remind:%d:4", eventId))
	kb.AddRow().AddCallback("⏰ За 12 часов", schemes.DEFAULT, fmt.Sprintf("reg_remind:%d:12", eventId))
	kb.AddRow().AddCallback("⏰ За 24 часа", schemes.DEFAULT, fmt.Sprintf("reg_remind:%d:24", eventId))
	kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, fmt.Sprintf("view_event:%d", eventId))
	answerCb(ctx, api, chatId, cbId, "🔔 За сколько часов напомнить о мероприятии?", kb)
}

func confirmEventRegistration(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId, eventId int64, remindHours int, cbId string) {
	var regExists int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM event_registrations WHERE event_id=$1 AND user_id=$2", eventId, userId).Scan(&regExists)
	if regExists > 0 {
		answerCb(ctx, api, chatId, cbId, "ℹ️ Вы уже записаны на это мероприятие.", nil)
		return
	}
	var maxPart, curPart int
	var title string
	var eventDate time.Time
	err := pool.QueryRow(ctx, "SELECT title, event_date, max_participants, current_participants FROM events WHERE id=$1 AND is_active=true AND event_date::date >= CURRENT_DATE", eventId).
		Scan(&title, &eventDate, &maxPart, &curPart)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Мероприятие не найдено.", nil)
		return
	}
	if maxPart > 0 && curPart >= maxPart {
		answerCb(ctx, api, chatId, cbId, "❌ Мест нет.", nil)
		return
	}
	tag, err := pool.Exec(ctx,
		"INSERT INTO event_registrations (event_id, user_id, chat_id, remind_hours) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING",
		eventId, userId, chatId, remindHours)
	if err != nil {
		log.Printf("confirmEventRegistration insert error: %v", err)
		answerCb(ctx, api, chatId, cbId, "❌ Ошибка записи.", nil)
		return
	}
	if tag.RowsAffected() == 0 {
		answerCb(ctx, api, chatId, cbId, "ℹ️ Вы уже записаны на это мероприятие.", nil)
		return
	}
	pool.Exec(ctx, "UPDATE events SET current_participants=current_participants+1 WHERE id=$1", eventId)
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("📅 Все мероприятия", schemes.DEFAULT, "events_list:0")
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	answerCb(ctx, api, chatId, cbId, fmt.Sprintf(
		"🎉 Вы записаны!\n━━━━━━━━━━━━━━━━━━━━\n\n🎭 %s\n📅 %s\n🔔 Напомним за %d ч.\n\nПриходите вовремя!",
		title, eventDate.Format("02.01.2006 в 15:04"), remindHours), kb)
}

func unregisterFromEvent(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId, eventId int64, cbId string) {
	tag, err := pool.Exec(ctx, "DELETE FROM event_registrations WHERE event_id=$1 AND user_id=$2", eventId, userId)
	if err != nil || tag.RowsAffected() == 0 {
		answerCb(ctx, api, chatId, cbId, "ℹ️ Вы не были записаны на это мероприятие.", nil)
		return
	}
	pool.Exec(ctx, "UPDATE events SET current_participants=GREATEST(current_participants-1,0) WHERE id=$1", eventId)
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("📅 Все мероприятия", schemes.DEFAULT, "events_list:0")
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.NEGATIVE, "main")
	answerCb(ctx, api, chatId, cbId, "✅ Запись отменена.", kb)
}

func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371
	lat1R := lat1 * math.Pi / 180
	lat2R := lat2 * math.Pi / 180
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1R)*math.Cos(lat2R)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

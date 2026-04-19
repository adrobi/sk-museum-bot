package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

func showAddMuseum(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, cbId string) {
	setUserState(userId, "awaiting_museum_data", "")
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("❌ Отмена", schemes.NEGATIVE, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId,
		"➕ Добавление музея\n━━━━━━━━━━━━━━━━━━━━\nОтправьте данные:\n\nНазвание\nКраткое название\nОписание\nАдрес\nТелефон\nEmail\nСайт\nЧасы работы", kb)
}

func showMuseumManageList(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, role, cbId string) {
	var rows interface{ Next() bool }
	var err error
	if role == RoleBotAdmin {
		r, e := pool.Query(ctx, "SELECT id, COALESCE(NULLIF(short_name,''), name) FROM museums ORDER BY name")
		rows, err = r, e
	} else {
		r, e := pool.Query(ctx,
			"SELECT m.id, COALESCE(NULLIF(m.short_name,''), m.name) FROM museums m JOIN staff s ON m.id=s.museum_id WHERE s.user_id=$1 ORDER BY m.name", userId)
		rows, err = r, e
	}
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Ошибка загрузки.", nil)
		return
	}
	type scannable interface{ Scan(dest ...any) error }
	kb := api.Messages.NewKeyboardBuilder()
	count := 0
	for rows.Next() {
		var id int64
		var name string
		if rows.(scannable).Scan(&id, &name) == nil {
			kb.AddRow().AddCallback(name, schemes.DEFAULT, fmt.Sprintf("adm:manage_mus:%d", id))
			count++
		}
	}
	if closer, ok := rows.(interface{ Close() }); ok {
		closer.Close()
	}
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	if count == 0 {
		answerCb(ctx, api, chatId, cbId, "📋 Нет доступных музеев.", kb)
		return
	}
	answerCb(ctx, api, chatId, cbId, fmt.Sprintf("🏛 Управление музеями (%d)\n━━━━━━━━━━━━━━━━━━━━\n\nВыберите музей:", count), kb)
}

func showMuseumManage(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, role string, museumId int64, cbId string) {
	if !canAccessMuseum(ctx, pool, userId, role, museumId) {
		answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа к этому музею.", nil)
		return
	}
	var name string
	pool.QueryRow(ctx, "SELECT COALESCE(NULLIF(short_name,''), name) FROM museums WHERE id=$1", museumId).Scan(&name)
	kb := api.Messages.NewKeyboardBuilder()

	if role == RoleBotAdmin || role == RoleMuseumAdmin {
		kb.AddRow().AddCallback("✏️ Редактировать описание", schemes.DEFAULT, fmt.Sprintf("adm:edit_mus:%d", museumId))
		kb.AddRow().AddCallback("📷 Изменить фото музея", schemes.DEFAULT, fmt.Sprintf("adm:set_photo_mus:%d", museumId))
	}
	if role == RoleBotAdmin {
		kb.AddRow().AddCallback("🗑 Удалить музей", schemes.NEGATIVE, fmt.Sprintf("adm:del_mus:%d", museumId))
	}

	// AI / Интерактивный музей
	kb.AddRow().AddCallback("🤖 Обучить модель AI", schemes.POSITIVE, fmt.Sprintf("adm:train_model:%d", museumId))
	kb.AddRow().AddLink("🔬 Открыть интерактивный музей", schemes.POSITIVE,
		fmt.Sprintf("%s/?museum_id=%d", getWebAppURL(), museumId))

	// Кнопки добавления
	if role == RoleBotAdmin || role == RoleMuseumAdmin || role == RoleContentManager {
		kb.AddRow().AddCallback("➕ Добавить выставку", schemes.POSITIVE, fmt.Sprintf("add_exh_mus:%d", museumId))
		kb.AddRow().AddCallback("➕ Добавить мероприятие", schemes.POSITIVE, fmt.Sprintf("add_event_mus:%d", museumId))
	}

	// Выставки этого музея
	exRows, err := pool.Query(ctx, "SELECT id, title FROM exhibitions WHERE museum_id=$1 ORDER BY title", museumId)
	if err == nil {
		for exRows.Next() {
			var eid int64
			var title string
			if exRows.Scan(&eid, &title) == nil {
				kb.AddRow().AddCallback("🎨 "+title, schemes.POSITIVE, fmt.Sprintf("adm:manage_exbn:%d", eid))
			}
		}
		exRows.Close()
	}

	// Мероприятия
	evRows, err := pool.Query(ctx, "SELECT id, title FROM events WHERE museum_id=$1 AND is_active=true ORDER BY event_date", museumId)
	if err == nil {
		for evRows.Next() {
			var eid int64
			var title string
			if evRows.Scan(&eid, &title) == nil {
				kb.AddRow().AddCallback("📅 "+title+" 🗑", schemes.NEGATIVE, fmt.Sprintf("adm:del_event:%d", eid))
			}
		}
		evRows.Close()
	}

	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId, fmt.Sprintf("⚙️ Управление: %s\n━━━━━━━━━━━━━━━━━━━━\n\nВыберите действие:", name), kb)
}

func showExhibitionManage(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, role string, exbnId int64, cbId string) {
	var title string
	var museumId int64
	err := pool.QueryRow(ctx, "SELECT title, museum_id FROM exhibitions WHERE id=$1", exbnId).Scan(&title, &museumId)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Выставка не найдена.", nil)
		return
	}
	if !canAccessMuseum(ctx, pool, userId, role, museumId) {
		answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
		return
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("📷 Изменить фото выставки", schemes.DEFAULT, fmt.Sprintf("adm:set_photo_exh:%d", exbnId))
	kb.AddRow().AddCallback("🗑 Удалить выставку", schemes.NEGATIVE, fmt.Sprintf("adm:del_exbn:%d", exbnId))
	kb.AddRow().AddCallback("➕ Добавить экспонат", schemes.POSITIVE, fmt.Sprintf("adm:add_exbt:%d", exbnId))

	// Экспонаты
	exRows, err := pool.Query(ctx, "SELECT id, title FROM exhibits WHERE exhibition_id=$1 ORDER BY title", exbnId)
	if err == nil {
		for exRows.Next() {
			var eid int64
			var t string
			if exRows.Scan(&eid, &t) == nil {
				row := kb.AddRow()
				row.AddCallback("🖼 "+t, schemes.DEFAULT, fmt.Sprintf("view_exbt:%d", eid))
				row.AddCallback("📷", schemes.DEFAULT, fmt.Sprintf("adm:set_photo_exbt:%d", eid))
				row.AddCallback("🗑", schemes.NEGATIVE, fmt.Sprintf("adm:del_exbt:%d", eid))
			}
		}
		exRows.Close()
	}

	kb.AddRow().AddCallback("🏛 К музею", schemes.DEFAULT, fmt.Sprintf("adm:manage_mus:%d", museumId))
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId, fmt.Sprintf("🎨 Управление: %s\n━━━━━━━━━━━━━━━━━━━━\n\nВыберите действие:", title), kb)
}

func showAddExhibition(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, role, cbId string) {
	if role != RoleBotAdmin && role != RoleMuseumAdmin && role != RoleContentManager {
		answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
		return
	}
	var queryStr string
	var args []interface{}
	if role == RoleBotAdmin {
		queryStr = "SELECT id, COALESCE(NULLIF(short_name,''), name) FROM museums ORDER BY name"
	} else {
		queryStr = "SELECT m.id, COALESCE(NULLIF(m.short_name,''), m.name) FROM museums m JOIN staff s ON m.id=s.museum_id WHERE s.user_id=$1 ORDER BY m.name"
		args = append(args, userId)
	}
	rows, err := pool.Query(ctx, queryStr, args...)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Ошибка.", nil)
		return
	}
	defer rows.Close()
	kb := api.Messages.NewKeyboardBuilder()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			kb.AddRow().AddCallback(name, schemes.POSITIVE, fmt.Sprintf("add_exh_mus:%d", id))
		}
	}
	kb.AddRow().AddCallback("🛡 Назад", schemes.NEGATIVE, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId, "🎨 Добавление выставки\n━━━━━━━━━━━━━━━━━━━━\n\nВыберите музей:", kb)
}

func showAddEvent(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, role, cbId string) {
	if role != RoleBotAdmin && role != RoleMuseumAdmin && role != RoleContentManager {
		answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
		return
	}
	var queryStr string
	var args []interface{}
	if role == RoleBotAdmin {
		queryStr = "SELECT id, COALESCE(NULLIF(short_name,''), name) FROM museums ORDER BY name"
	} else {
		queryStr = "SELECT m.id, COALESCE(NULLIF(m.short_name,''), m.name) FROM museums m JOIN staff s ON m.id=s.museum_id WHERE s.user_id=$1 ORDER BY m.name"
		args = append(args, userId)
	}
	rows, err := pool.Query(ctx, queryStr, args...)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Ошибка.", nil)
		return
	}
	defer rows.Close()
	kb := api.Messages.NewKeyboardBuilder()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil {
			kb.AddRow().AddCallback(name, schemes.POSITIVE, fmt.Sprintf("add_event_mus:%d", id))
		}
	}
	kb.AddRow().AddCallback("🛡 Назад", schemes.NEGATIVE, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId, "📅 Добавление мероприятия\n━━━━━━━━━━━━━━━━━━━━\n\nВыберите музей:", kb)
}

func showStaffManagement(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId int64, role, cbId string) {
	if role != RoleBotAdmin {
		answerCb(ctx, api, chatId, cbId, "⛔ Только главный администратор.", nil)
		return
	}
	rows, err := pool.Query(ctx, `
		SELECT s.user_id, COALESCE(s.alias,'—'), s.role, s.email, COALESCE(m.short_name,'все музеи')
		FROM staff s LEFT JOIN museums m ON s.museum_id=m.id ORDER BY s.role, s.alias`)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Ошибка.", nil)
		return
	}
	defer rows.Close()
	roleEmoji := map[string]string{"bot_admin": "👑", "museum_admin": "🏛", "content_manager": "📝", "analyst": "📊"}
	roleNames := map[string]string{"bot_admin": "Гл. админ", "museum_admin": "Админ", "content_manager": "Контент", "analyst": "Аналитик"}
	text := "👥 Управление персоналом\n━━━━━━━━━━━━━━━━━━━━\n\n"
	kb := api.Messages.NewKeyboardBuilder()
	for rows.Next() {
		var uid int64
		var alias, sRole, email, museum string
		if rows.Scan(&uid, &alias, &sRole, &email, &museum) == nil {
			text += fmt.Sprintf("%s %s\n   %s | 🏛 %s\n   📧 %s | 🆔 %d\n\n", roleEmoji[sRole], alias, roleNames[sRole], museum, email, uid)
			row := kb.AddRow()
			row.AddCallback("✏️ "+alias, schemes.DEFAULT, fmt.Sprintf("adm:edit_staff:%d", uid))
			row.AddCallback("🗑", schemes.NEGATIVE, fmt.Sprintf("adm:del_staff:%d", uid))
		}
	}
	kb.AddRow().AddCallback("➕ Добавить сотрудника", schemes.POSITIVE, "adm:add_staff")
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId, text, kb)
}

func showAnalytics(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, role, cbId string) {
	if role != RoleBotAdmin && role != RoleMuseumAdmin && role != RoleAnalyst {
		answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
		return
	}
	var totalMuseums, totalExhibitions, totalEvents, totalReviews int
	var totalSearches int64
	var avgRating *float64
	if role == RoleBotAdmin || role == RoleAnalyst {
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM museums").Scan(&totalMuseums)
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM exhibitions").Scan(&totalExhibitions)
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM events WHERE is_active=true").Scan(&totalEvents)
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM search_logs WHERE created_at > NOW()-INTERVAL '30 days'").Scan(&totalSearches)
		pool.QueryRow(ctx, "SELECT COUNT(*), AVG(rating)::numeric(3,1) FROM reviews").Scan(&totalReviews, &avgRating)
	} else {
		ids := getStaffMuseumIds(ctx, pool, userId)
		if len(ids) == 0 {
			answerCb(ctx, api, chatId, cbId, "📊 Нет данных для ваших музеев.", nil)
			return
		}
		idStr := intSliceToStr(ids)
		pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM museums WHERE id IN (%s)", idStr)).Scan(&totalMuseums)
		pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM exhibitions WHERE museum_id IN (%s)", idStr)).Scan(&totalExhibitions)
		pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM events WHERE museum_id IN (%s) AND is_active=true", idStr)).Scan(&totalEvents)
		pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*), AVG(rating)::numeric(3,1) FROM reviews WHERE museum_id IN (%s)", idStr)).Scan(&totalReviews, &avgRating)
	}
	text := fmt.Sprintf("📊 Аналитика\n━━━━━━━━━━━━━━━━━━━━\n\n🏛 Музеев: %d\n🎨 Выставок: %d\n📅 Мероприятий: %d\n💬 Отзывов: %d\n",
		totalMuseums, totalExhibitions, totalEvents, totalReviews)
	if totalSearches > 0 {
		text += fmt.Sprintf("🔍  Поисков (30 дн.): %d\n", totalSearches)
	}
	if avgRating != nil {
		text += fmt.Sprintf("⭐  Ср. рейтинг: %.1f\n", *avgRating)
	}
	if role == RoleBotAdmin || role == RoleAnalyst {
		rows, err := pool.Query(ctx, `SELECT query, COUNT(*) FROM search_logs WHERE created_at > NOW()-INTERVAL '30 days' GROUP BY query ORDER BY COUNT(*) DESC LIMIT 5`)
		if err == nil {
			defer rows.Close()
			text += "\n🔥 Топ запросов:\n"
			i := 1
			for rows.Next() {
				var q string
				var c int
				if rows.Scan(&q, &c) == nil {
					text += fmt.Sprintf("  %d. %s (%d)\n", i, q, c)
					i++
				}
			}
			if i == 1 {
				text += "  Пока нет данных\n"
			}
		}
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId, text, kb)
}

func showReviews(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, userId int64, role, cbId string) {
	if role != RoleBotAdmin && role != RoleMuseumAdmin && role != RoleAnalyst {
		answerCb(ctx, api, chatId, cbId, "⛔ Нет доступа.", nil)
		return
	}
	var query string
	var args []interface{}
	if role == RoleBotAdmin || role == RoleAnalyst {
		query = `SELECT r.rating, COALESCE(r.comment,''), COALESCE(m.short_name,m.name), r.created_at
			FROM reviews r JOIN museums m ON r.museum_id=m.id ORDER BY r.created_at DESC LIMIT 15`
	} else {
		query = `SELECT r.rating, COALESCE(r.comment,''), COALESCE(m.short_name,m.name), r.created_at
			FROM reviews r JOIN museums m ON r.museum_id=m.id JOIN staff s ON m.id=s.museum_id
			WHERE s.user_id=$1 ORDER BY r.created_at DESC LIMIT 15`
		args = append(args, userId)
	}
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		answerCb(ctx, api, chatId, cbId, "❌ Ошибка.", nil)
		return
	}
	defer rows.Close()
	text := "💬 Последние отзывы\n━━━━━━━━━━━━━━━━━━━━\n\n"
	count := 0
	for rows.Next() {
		var rating int
		var comment, museum string
		var createdAt time.Time
		if rows.Scan(&rating, &comment, &museum, &createdAt) == nil {
			if comment == "" {
				comment = "без комментария"
			}
			text += fmt.Sprintf("🏛 %s\n   %s%s\n   💬 %s\n   📅 %s\n\n",
				museum, strings.Repeat("⭐", rating), strings.Repeat("☆", 5-rating), comment, createdAt.Format("02.01.2006 15:04"))
			count++
		}
	}
	if count == 0 {
		text += "Пока нет отзывов.\n"
	}
	kb := api.Messages.NewKeyboardBuilder()
	kb.AddRow().AddCallback("🛡 Панель управления", schemes.DEFAULT, "adm:admin_menu")
	answerCb(ctx, api, chatId, cbId, text, kb)
}

func intSliceToStr(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}

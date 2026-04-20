package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	maxbot "github.com/max-messenger/max-bot-api-client-go"
	"github.com/max-messenger/max-bot-api-client-go/schemes"
)

func showExhibitionDetails(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, id, userId int64) {
	var e struct {
		title      string
		desc, img  *string
		st, en     *string
		museumName *string
		museumId   *int64
	}
	err := pool.QueryRow(ctx, `
		SELECT ex.title, ex.description, ex.image_url,
		       ex.start_date::text, ex.end_date::text,
		       m.short_name, m.id
		FROM exhibitions ex JOIN museums m ON ex.museum_id=m.id WHERE ex.id=$1`, id).
		Scan(&e.title, &e.desc, &e.img, &e.st, &e.en, &e.museumName, &e.museumId)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Выставка не найдена."))
		return
	}
	text := fmt.Sprintf("🎨 %s\n━━━━━━━━━━━━━━━━━━━━\n\n", e.title)
	if e.museumName != nil {
		text += fmt.Sprintf("🏛 %s\n", *e.museumName)
	}
	if e.desc != nil && *e.desc != "" {
		text += fmt.Sprintf("\n📝 %s\n", *e.desc)
	}
	if e.st != nil && e.en != nil {
		text += fmt.Sprintf("\n📅 %s — %s\n\n", *e.st, *e.en)
	}
	kb := api.Messages.NewKeyboardBuilder()
	exRows, err := pool.Query(ctx,
		"SELECT id, title FROM exhibits WHERE exhibition_id=$1 AND is_active=true ORDER BY title LIMIT 15", id)
	if err == nil {
		first := true
		for exRows.Next() {
			if first {
				text += "\n🖼 ЭКСПОНАТЫ:"
				first = false
			}
			var eid int64
			var title string
			if exRows.Scan(&eid, &title) == nil {
				kb.AddRow().AddCallback("  🔹 "+title, schemes.POSITIVE, fmt.Sprintf("view_exbt:%d", eid))
			}
		}
		exRows.Close()
	}
	if e.museumId != nil {
		kb.AddRow().AddCallback("🏛 К музею", schemes.POSITIVE, fmt.Sprintf("view_mus:%d", *e.museumId))
	}
	session := getSession(userId)
	if session != nil && e.museumId != nil {
		r := session.Role
		if canAccessMuseum(ctx, pool, userId, r, *e.museumId) &&
			(r == RoleBotAdmin || r == RoleMuseumAdmin || r == RoleContentManager) {
			kb.AddRow().AddCallback("⚙️ Управление выставкой", schemes.DEFAULT, fmt.Sprintf("adm:manage_exbn:%d", id))
		}
	}
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.POSITIVE, "main")
	msg := maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text)
	hasPhoto := false
	if e.img != nil && *e.img != "" {
		hasPhoto = attachPhoto(ctx, api, msg, *e.img)
	}
	if err := api.Messages.Send(ctx, msg); err != nil {
		log.Printf("showExhibitionDetails send error: %v", err)
		if hasPhoto {
			fallback := maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text)
			if retryErr := api.Messages.Send(ctx, fallback); retryErr != nil {
				log.Printf("showExhibitionDetails fallback send error: %v", retryErr)
			}
		}
	}
}

func showExhibitDetails(ctx context.Context, api *maxbot.Api, pool *pgxpool.Pool, chatId, id int64) {
	var title string
	var desc, img *string
	var exbnTitle *string
	var exbnId *int64
	err := pool.QueryRow(ctx, `
		SELECT e.title, e.description, e.image_url, ex.title, ex.id
		FROM exhibits e JOIN exhibitions ex ON e.exhibition_id=ex.id WHERE e.id=$1`, id).
		Scan(&title, &desc, &img, &exbnTitle, &exbnId)
	if err != nil {
		_ = api.Messages.Send(ctx, maxbot.NewMessage().SetChat(chatId).SetText("❌ Экспонат не найден."))
		return
	}
	text := fmt.Sprintf("🖼 %s\n━━━━━━━━━━━━━━━━━━━━\n\n", title)
	if exbnTitle != nil {
		text += fmt.Sprintf("🎨 Выставка: %s\n", *exbnTitle)
	}
	catRows, err := pool.Query(ctx,
		"SELECT c.name FROM exhibit_categories c JOIN exhibit_category_links l ON c.id=l.category_id WHERE l.exhibit_id=$1", id)
	if err == nil {
		var cats []string
		for catRows.Next() {
			var cat string
			if catRows.Scan(&cat) == nil {
				cats = append(cats, cat)
			}
		}
		catRows.Close()
		if len(cats) > 0 {
			text += fmt.Sprintf("🏷️ %s\n", strings.Join(cats, ", "))
		}
	}
	if desc != nil && *desc != "" {
		text += fmt.Sprintf("\n📝 %s\n", *desc)
	}
	kb := api.Messages.NewKeyboardBuilder()
	if exbnId != nil {
		kb.AddRow().AddCallback("🎨 К выставке", schemes.POSITIVE, fmt.Sprintf("view_exbn:%d", *exbnId))
	}
	kb.AddRow().AddCallback("🏠 Главное меню", schemes.POSITIVE, "main")
	msg := maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text)
	hasPhoto := false
	if img != nil && *img != "" {
		hasPhoto = attachPhoto(ctx, api, msg, *img)
	}
	if err := api.Messages.Send(ctx, msg); err != nil {
		log.Printf("showExhibitDetails send error: %v", err)
		if hasPhoto {
			fallback := maxbot.NewMessage().SetChat(chatId).AddKeyboard(kb).SetText(text)
			if retryErr := api.Messages.Send(ctx, fallback); retryErr != nil {
				log.Printf("showExhibitDetails fallback send error: %v", retryErr)
			}
		}
	}
}

func attachPhoto(ctx context.Context, api *maxbot.Api, msg *maxbot.Message, imgStr string) bool {
	imageRef := strings.TrimSpace(imgStr)
	if imageRef == "" {
		return false
	}

	lower := strings.ToLower(imageRef)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		photos, err := api.Uploads.UploadPhotoFromUrl(ctx, imageRef)
		if err != nil {
			log.Printf("Фото (url) не прикреплено: %v", err)
			return false
		}
		for _, pt := range photos.Photos {
			if pt.Token != "" {
				msg.AddPhotoByToken(pt.Token)
				return true
			}
		}
		log.Printf("Фото (url) не прикреплено: пустой token в PhotoTokens")
		return false
	}

	if !looksLikeAttachmentToken(imageRef) {
		log.Printf("Фото (token) пропущено: неподдерживаемый image_ref=%q", imageRef)
		return false
	}

	msg.AddPhotoByToken(imageRef)
	return true
}

func looksLikeAttachmentToken(ref string) bool {
	if ref == "" {
		return false
	}
	if strings.ContainsAny(ref, " \t\r\n") {
		return false
	}
	if strings.Contains(ref, "://") {
		return false
	}
	if strings.Contains(ref, "/") || strings.Contains(ref, `\\`) {
		return false
	}
	return true
}

-- Дополнительные таблицы, которые нужно добавить в init.sql

-- Таблица участников мероприятий
CREATE TABLE IF NOT EXISTS event_participants (
    id SERIAL PRIMARY KEY,
    event_id INTEGER REFERENCES events(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    registered_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'registered', -- registered, cancelled, attended
    UNIQUE(event_id, user_id)
);

-- Индексы для оптимизации поиска
CREATE INDEX IF NOT EXISTS idx_museums_coords ON museums(latitude, longitude);
CREATE INDEX IF NOT EXISTS idx_exhibits_exhibition ON exhibits(exhibition_id);
CREATE INDEX IF NOT EXISTS idx_events_date ON events(event_date);
CREATE INDEX IF NOT EXISTS idx_reviews_museum ON reviews(museum_id);
CREATE INDEX IF NOT EXISTS idx_search_logs_created ON search_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_staff_user ON staff(user_id);
CREATE INDEX IF NOT EXISTS idx_staff_role ON staff(role);

-- Триггер для обновления времени последнего входа
CREATE OR REPLACE FUNCTION update_last_login()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE staff 
    SET last_login = CURRENT_TIMESTAMP 
    WHERE user_id = NEW.user_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Создаем триггер (можно создать вручную после добавления таблиц)
-- CREATE TRIGGER trigger_update_last_login
--     AFTER INSERT ON event_participants
--     FOR EACH ROW
--     EXECUTE FUNCTION update_last_login();

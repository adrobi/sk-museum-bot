import { useState, useEffect } from "react";
import { MapPin, Search, Loader2, Navigation, Map } from "lucide-react";
import { fetchMuseums, fetchNearbyMuseums, fetchMuseumStatus } from "../api";

function normalizeText(value) {
  return String(value || "")
    .toLowerCase()
    .replace(/ё/g, "е")
    .replace(/\s+/g, " ")
    .trim();
}

export default function MuseumSelect({ onSelect, onOpenMap, bridge }) {
  const [museums, setMuseums] = useState([]);
  const [filtered, setFiltered] = useState([]);
  const [loading, setLoading] = useState(true);
  const [geoLoading, setGeoLoading] = useState(false);
  const [query, setQuery] = useState("");
  const [error, setError] = useState("");
  const [modelStatuses, setModelStatuses] = useState({});

  useEffect(() => {
    fetchMuseums()
      .then((data) => {
        setMuseums(data);
        setFiltered(data);
      })
      .catch(() => setError("Не удалось загрузить список музеев"))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    const normalizedQuery = normalizeText(query);
    if (!normalizedQuery) {
      setFiltered(museums);
      return;
    }

    const tokens = normalizedQuery.split(" ").filter(Boolean);
    setFiltered(
      museums.filter((m) => {
        const haystack = normalizeText(
          [m.name, m.display_name, m.address, m.description].join(" ")
        );
        return tokens.every((token) => haystack.includes(token));
      })
    );
  }, [query, museums]);

  async function handleSelect(museum) {
    // Проверяем статус модели
    if (!modelStatuses[museum.id]) {
      try {
        const status = await fetchMuseumStatus(museum.id);
        setModelStatuses((prev) => ({ ...prev, [museum.id]: status }));
        if (!status.model_ready) {
          // Показываем предупреждение, но всё равно можно войти
        }
      } catch {}
    }
    onSelect(museum);
  }

  async function handleGeo() {
    setError("");

    if (!window.isSecureContext) {
      setError("Геолокация работает только в защищённом окружении (HTTPS внутри MAX)");
      return;
    }

    if (!navigator.geolocation) {
      setError("Геолокация не поддерживается");
      return;
    }

    if (navigator.permissions?.query) {
      try {
        const permission = await navigator.permissions.query({ name: "geolocation" });
        if (permission.state === "denied") {
          setError(
            bridge?.isMiniApp
              ? "Доступ к геолокации запрещён. Разрешите его в настройках мини-приложения MAX и попробуйте снова"
              : "Доступ к геолокации запрещён в настройках браузера"
          );
          return;
        }
      } catch {
      }
    }

    setGeoLoading(true);
    navigator.geolocation.getCurrentPosition(
      async ({ coords }) => {
        try {
          const nearby = await fetchNearbyMuseums(coords.latitude, coords.longitude);
          if (nearby.length > 0) {
            setMuseums(nearby);
            setFiltered(nearby);
          } else {
            setError("Музеи поблизости не найдены");
          }
        } catch {
          setError("Ошибка определения ближайших музеев");
        } finally {
          setGeoLoading(false);
        }
      },
      (geoError) => {
        if (geoError?.code === 1) {
          setError(
            bridge?.isMiniApp
              ? "MAX не дал доступ к геолокации. Проверьте разрешение для мини-приложения в настройках MAX"
              : "Нет доступа к геолокации"
          );
        } else if (geoError?.code === 2) {
          setError("Не удалось определить местоположение. Проверьте GPS/интернет");
        } else if (geoError?.code === 3) {
          setError("Не удалось получить геолокацию вовремя. Повторите попытку");
        } else {
          setError("Не удалось получить геолокацию");
        }
        setGeoLoading(false);
      },
      {
        enableHighAccuracy: true,
        timeout: 15000,
        maximumAge: 0,
      }
    );
  }

  return (
    <div className="p-4 max-w-lg mx-auto space-y-4">
      <div className="text-center pt-4 pb-2">
        <div className="text-5xl mb-3">🏛</div>
        <h2 className="text-xl font-bold text-stone-100">Выберите музей</h2>
        <p className="text-stone-400 text-sm mt-1">
          Направьте камеру на экспонат для его определения
        </p>
      </div>

      {/* Геолокация и карта */}
      <div className="flex gap-2">
        <button
          onClick={handleGeo}
          disabled={geoLoading}
          className="flex-1 flex items-center justify-center gap-2 py-2.5 rounded-xl border border-museum-500/50 text-museum-400 hover:bg-museum-500/10 transition-colors text-sm font-medium disabled:opacity-50"
        >
          {geoLoading ? (
            <Loader2 size={16} className="animate-spin" />
          ) : (
            <Navigation size={16} />
          )}
          {geoLoading ? "Определяем..." : "Ближайшие"}
        </button>
        <button
          onClick={() => onOpenMap(museums)}
          disabled={loading || museums.length === 0}
          className="flex-1 flex items-center justify-center gap-2 py-2.5 rounded-xl border border-museum-500/50 text-museum-400 hover:bg-museum-500/10 transition-colors text-sm font-medium disabled:opacity-50"
        >
          <Map size={16} />
          Выбрать на карте
        </button>
      </div>

      {/* Поиск */}
      <div className="relative">
        <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-stone-500" />
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Поиск по названию, адресу, описанию..."
          className="w-full bg-stone-800 border border-stone-700 rounded-xl pl-9 pr-4 py-2.5 text-sm text-stone-100 placeholder-stone-500 focus:outline-none focus:border-museum-500 transition-colors"
        />
      </div>

      {error && (
        <p className="text-red-400 text-sm text-center">{error}</p>
      )}

      {/* Список */}
      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 size={32} className="animate-spin text-museum-400" />
        </div>
      ) : (
        <div className="space-y-2">
          {filtered.length === 0 && (
            <p className="text-center text-stone-500 py-8">Ничего не найдено</p>
          )}
          {filtered.map((m) => (
            <button
              key={m.id}
              onClick={() => handleSelect(m)}
              className="w-full card p-3.5 text-left hover:border-museum-500/50 hover:bg-stone-800/60 transition-all group"
            >
              <div className="flex items-start gap-3">
                {m.image_url ? (
                  <img
                    src={m.image_url}
                    alt=""
                    className="w-12 h-12 rounded-lg object-cover shrink-0 bg-stone-800"
                    onError={(e) => {
                      e.currentTarget.style.display = "none";
                      e.currentTarget.nextSibling.style.display = "flex";
                    }}
                  />
                ) : null}
                <div
                  className="w-12 h-12 rounded-lg bg-stone-800 items-center justify-center shrink-0 text-xl"
                  style={{ display: m.image_url ? "none" : "flex" }}
                >
                  🏛
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-stone-100 text-sm leading-snug group-hover:text-museum-300 transition-colors line-clamp-2">
                    {m.name}
                  </p>
                  {m.display_name && m.display_name !== m.name && (
                    <p className="text-stone-500 text-xs mt-0.5 truncate">
                      {m.display_name}
                    </p>
                  )}
                  {m.address && (
                    <p className="text-stone-500 text-xs mt-0.5 flex items-center gap-1">
                      <MapPin size={10} />
                      {m.address}
                    </p>
                  )}
                  {m.distance_km !== undefined && (
                    <span className="inline-block mt-1 text-xs bg-museum-500/20 text-museum-300 px-2 py-0.5 rounded-full">
                      {m.distance_km} км
                    </span>
                  )}
                </div>
                <span className="text-stone-600 group-hover:text-museum-400 transition-colors text-lg">›</span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

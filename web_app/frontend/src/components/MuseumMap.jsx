import { useEffect, useRef, useState } from "react";
import { ArrowLeft, Loader2, MapPin } from "lucide-react";

export default function MuseumMap({ museums, onSelect, onBack }) {
  const mapRef = useRef(null);
  const mapInstanceRef = useRef(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!window.ymaps) {
      setError("Яндекс Карты не загружены. Проверьте подключение к интернету.");
      return;
    }

    window.ymaps.ready(() => {
      if (mapInstanceRef.current) {
        mapInstanceRef.current.destroy();
      }

      // Center on Stavropol region
      const map = new window.ymaps.Map(mapRef.current, {
        center: [44.95, 42.73],
        zoom: 7,
        controls: ["zoomControl", "geolocationControl"],
      });

      mapInstanceRef.current = map;

      // Add museum placemarks
      const museumsWithCoords = museums.filter(
        (m) => m.latitude && m.longitude
      );

      museumsWithCoords.forEach((m) => {
        const placemark = new window.ymaps.Placemark(
          [m.latitude, m.longitude],
          {
            balloonContentHeader: `<strong style="font-size:14px">${m.name}</strong>`,
            balloonContentBody: `
              <div style="max-width:250px">
                ${m.address ? `<p style="color:#666;margin:4px 0;font-size:12px">📍 ${m.address}</p>` : ""}
                ${m.description ? `<p style="margin:6px 0;font-size:13px">${m.description.slice(0, 150)}${m.description.length > 150 ? "..." : ""}</p>` : ""}
                <button 
                  id="select-museum-${m.id}" 
                  style="margin-top:8px;padding:6px 16px;background:#d4842a;color:white;border:none;border-radius:8px;cursor:pointer;font-size:13px;font-weight:500"
                >
                  Выбрать музей
                </button>
              </div>
            `,
            hintContent: m.display_name || m.name,
          },
          {
            preset: "islands#museumIcon",
            iconColor: "#d4842a",
          }
        );

        placemark.events.add("balloonopen", () => {
          // Wait for balloon DOM to render
          setTimeout(() => {
            const btn = document.getElementById(`select-museum-${m.id}`);
            if (btn) {
              btn.onclick = () => {
                map.balloon.close();
                onSelect(m);
              };
            }
          }, 100);
        });

        map.geoObjects.add(placemark);
      });

      // Fit bounds to show all markers
      if (museumsWithCoords.length > 1) {
        map.setBounds(map.geoObjects.getBounds(), {
          checkZoomRange: true,
          zoomMargin: 40,
        });
      } else if (museumsWithCoords.length === 1) {
        map.setCenter(
          [museumsWithCoords[0].latitude, museumsWithCoords[0].longitude],
          12
        );
      }

      setReady(true);
    });

    return () => {
      if (mapInstanceRef.current) {
        mapInstanceRef.current.destroy();
        mapInstanceRef.current = null;
      }
    };
  }, [museums, onSelect]);

  return (
    <div className="flex flex-col h-[calc(100vh-56px)]">
      {/* Top bar */}
      <div className="flex items-center gap-3 px-4 py-3 bg-stone-900 border-b border-stone-800">
        <button
          onClick={onBack}
          className="p-1.5 text-stone-400 hover:text-stone-200 transition-colors"
        >
          <ArrowLeft size={20} />
        </button>
        <div className="flex items-center gap-2 text-stone-100 font-medium">
          <MapPin size={18} className="text-museum-400" />
          Музеи на карте
        </div>
        <span className="text-stone-500 text-xs ml-auto">
          {museums.filter((m) => m.latitude && m.longitude).length} на карте
        </span>
      </div>

      {/* Map container */}
      <div className="flex-1 relative">
        {error ? (
          <div className="absolute inset-0 flex items-center justify-center bg-stone-950">
            <p className="text-red-400 text-sm text-center px-6">{error}</p>
          </div>
        ) : (
          <>
            {!ready && (
              <div className="absolute inset-0 flex items-center justify-center bg-stone-950 z-10">
                <div className="flex flex-col items-center gap-3">
                  <Loader2 size={32} className="animate-spin text-museum-400" />
                  <p className="text-stone-400 text-sm">Загрузка карты...</p>
                </div>
              </div>
            )}
            <div ref={mapRef} className="w-full h-full" />
          </>
        )}
      </div>
    </div>
  );
}

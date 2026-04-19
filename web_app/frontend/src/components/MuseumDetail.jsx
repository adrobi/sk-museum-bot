import { useState, useEffect } from "react";
import {
  ArrowLeft,
  Camera,
  Loader2,
  MapPin,
  Clock,
  Phone,
  Globe,
  ChevronDown,
  ChevronUp,
  Image,
} from "lucide-react";
import { fetchMuseumDetail, fetchMuseumExhibitions } from "../api";

export default function MuseumDetail({ museum, onStartCamera, onBack }) {
  const [detail, setDetail] = useState(null);
  const [exhibitions, setExhibitions] = useState([]);
  const [loading, setLoading] = useState(true);
  const [expandedExh, setExpandedExh] = useState(null);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [d, exh] = await Promise.all([
          fetchMuseumDetail(museum.id),
          fetchMuseumExhibitions(museum.id),
        ]);
        setDetail(d);
        setExhibitions(exh);
      } catch {
        // fallback to what we already have
        setDetail(museum);
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [museum.id]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <Loader2 size={32} className="animate-spin text-museum-400" />
      </div>
    );
  }

  const info = detail || museum;

  return (
    <div className="pb-24">
      {/* Hero image */}
      <div className="relative">
        {info.image_url ? (
          <img
            src={info.image_url}
            alt={info.name}
            className="w-full h-56 object-cover"
            onError={(e) => {
              e.currentTarget.style.display = "none";
            }}
          />
        ) : (
          <div className="w-full h-40 bg-gradient-to-br from-stone-800 to-stone-900 flex items-center justify-center">
            <span className="text-6xl opacity-30">🏛</span>
          </div>
        )}
        <button
          onClick={onBack}
          className="absolute top-3 left-3 p-2 bg-black/50 backdrop-blur-sm rounded-full text-white hover:bg-black/70 transition-colors"
        >
          <ArrowLeft size={20} />
        </button>
      </div>

      {/* Museum info */}
      <div className="p-4 max-w-lg mx-auto space-y-4">
        <div>
          <h2 className="text-xl font-bold text-stone-100 leading-snug">
            {info.name}
          </h2>
          {info.display_name && info.display_name !== info.name && (
            <p className="text-museum-400 text-sm mt-1">{info.display_name}</p>
          )}
        </div>

        {info.description && (
          <p className="text-stone-400 text-sm leading-relaxed">
            {info.description}
          </p>
        )}

        {/* Contact info */}
        <div className="card p-4 space-y-3">
          {info.address && (
            <div className="flex items-start gap-3 text-sm">
              <MapPin size={16} className="text-museum-400 shrink-0 mt-0.5" />
              <span className="text-stone-300">{info.address}</span>
            </div>
          )}
          {info.opening_hours && (
            <div className="flex items-start gap-3 text-sm">
              <Clock size={16} className="text-museum-400 shrink-0 mt-0.5" />
              <span className="text-stone-300">{info.opening_hours}</span>
            </div>
          )}
          {info.phone && (
            <div className="flex items-start gap-3 text-sm">
              <Phone size={16} className="text-museum-400 shrink-0 mt-0.5" />
              <a
                href={`tel:${info.phone}`}
                className="text-stone-300 hover:text-museum-400 transition-colors"
              >
                {info.phone}
              </a>
            </div>
          )}
          {info.website && (
            <div className="flex items-start gap-3 text-sm">
              <Globe size={16} className="text-museum-400 shrink-0 mt-0.5" />
              <a
                href={info.website.startsWith("http") ? info.website : `http://${info.website}`}
                target="_blank"
                rel="noopener noreferrer"
                className="text-museum-400 hover:text-museum-300 transition-colors truncate"
              >
                {info.website}
              </a>
            </div>
          )}
        </div>

        {/* Exhibitions */}
        {exhibitions.length > 0 && (
          <div className="space-y-2">
            <h3 className="text-sm font-semibold uppercase tracking-wide text-museum-400 flex items-center gap-2">
              <Image size={14} />
              Выставки ({exhibitions.length})
            </h3>
            {exhibitions.map((exh) => (
              <div key={exh.id} className="card overflow-hidden">
                <button
                  onClick={() =>
                    setExpandedExh(expandedExh === exh.id ? null : exh.id)
                  }
                  className="w-full px-4 py-3 flex items-center gap-3 hover:bg-stone-800/50 transition-colors"
                >
                  {exh.image_url ? (
                    <img
                      src={exh.image_url}
                      alt=""
                      className="w-10 h-10 rounded-lg object-cover shrink-0"
                    />
                  ) : (
                    <div className="w-10 h-10 rounded-lg bg-stone-800 flex items-center justify-center shrink-0 text-lg">
                      🎨
                    </div>
                  )}
                  <div className="flex-1 min-w-0 text-left">
                    <p className="text-stone-100 text-sm font-medium truncate">
                      {exh.title}
                    </p>
                    {exh.start_date && (
                      <p className="text-stone-500 text-xs">
                        {exh.start_date}
                        {exh.end_date ? ` — ${exh.end_date}` : ""}
                      </p>
                    )}
                  </div>
                  {expandedExh === exh.id ? (
                    <ChevronUp size={16} className="text-stone-500 shrink-0" />
                  ) : (
                    <ChevronDown size={16} className="text-stone-500 shrink-0" />
                  )}
                </button>
                {expandedExh === exh.id && exh.description && (
                  <div className="px-4 pb-3 border-t border-stone-800">
                    <p className="text-stone-400 text-sm pt-3 leading-relaxed">
                      {exh.description}
                    </p>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Fixed bottom button */}
      <div className="fixed bottom-0 left-0 right-0 p-4 bg-gradient-to-t from-stone-950 via-stone-950 to-transparent pt-8">
        <button
          onClick={onStartCamera}
          className="w-full max-w-lg mx-auto flex items-center justify-center gap-3 py-3.5 bg-museum-500 hover:bg-museum-400 text-white rounded-2xl font-semibold text-base transition-colors shadow-lg shadow-museum-500/30"
        >
          <Camera size={22} />
          Определить экспонат
        </button>
      </div>
    </div>
  );
}

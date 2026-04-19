import { RotateCcw, Home, ChevronDown, ChevronUp } from "lucide-react";
import { useState } from "react";

export default function ExhibitResult({ results, museum, onRetry, onChangeMuseum }) {
  const [expanded, setExpanded] = useState(null);
  const top = results?.results?.[0];
  const others = results?.results?.slice(1) ?? [];

  const confidenceColor = (c) => {
    if (c >= 0.8) return "text-green-400";
    if (c >= 0.5) return "text-yellow-400";
    return "text-red-400";
  };

  const confidenceLabel = (c) => {
    if (c >= 0.8) return "Высокая уверенность";
    if (c >= 0.5) return "Средняя уверенность";
    return "Низкая уверенность";
  };

  if (!top) {
    return (
      <div className="p-6 text-center space-y-4">
        <div className="text-5xl">🔍</div>
        <p className="text-stone-300 font-medium">Экспонат не распознан</p>
        <p className="text-stone-500 text-sm">
          Попробуйте сделать фото ближе или при лучшем освещении
        </p>
        <div className="flex gap-3 justify-center pt-2">
          <button onClick={onRetry} className="btn-primary flex items-center gap-2">
            <RotateCcw size={16} /> Снова
          </button>
          <button onClick={onChangeMuseum} className="btn-secondary flex items-center gap-2">
            <Home size={16} /> Меню
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 max-w-lg mx-auto space-y-4">
      {/* Main result */}
      <div className="card p-4 space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium uppercase tracking-wide text-museum-400">
            Лучшее совпадение
          </span>
          <span className={`text-xs font-medium ${confidenceColor(top.confidence)}`}>
            {confidenceLabel(top.confidence)} · {Math.round(top.confidence * 100)}%
          </span>
        </div>

        {top.image_url && (
          <img
            src={top.image_url}
            alt={top.title}
            className="w-full h-48 object-cover rounded-xl bg-stone-800"
          />
        )}

        <div>
          <h2 className="text-lg font-bold text-stone-100">{top.title}</h2>
          {top.exhibition_title && (
            <p className="text-museum-400 text-sm mt-0.5">
              🎨 {top.exhibition_title}
            </p>
          )}
        </div>

        {top.description && (
          <p className="text-stone-400 text-sm leading-relaxed">{top.description}</p>
        )}

        {/* Confidence bar */}
        <div className="space-y-1">
          <div className="h-2 bg-stone-800 rounded-full overflow-hidden">
            <div
              className="h-full bg-museum-500 rounded-full transition-all"
              style={{ width: `${top.confidence * 100}%` }}
            />
          </div>
        </div>
      </div>

      {/* Other candidates */}
      {others.length > 0 && (
        <div className="card overflow-hidden">
          <button
            onClick={() => setExpanded(expanded === "others" ? null : "others")}
            className="w-full px-4 py-3 flex items-center justify-between text-sm text-stone-400 hover:text-stone-200 transition-colors"
          >
            <span>Другие варианты ({others.length})</span>
            {expanded === "others" ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
          </button>
          {expanded === "others" && (
            <div className="border-t border-stone-800 divide-y divide-stone-800">
              {others.map((r, i) => (
                <div key={r.exhibit_id} className="px-4 py-3 flex items-start gap-3">
                  <span className="text-stone-600 text-sm w-4 shrink-0">{i + 2}.</span>
                  <div className="flex-1 min-w-0">
                    <p className="text-stone-200 text-sm font-medium">{r.title}</p>
                    {r.exhibition_title && (
                      <p className="text-stone-500 text-xs">{r.exhibition_title}</p>
                    )}
                  </div>
                  <span className={`text-xs font-medium shrink-0 ${confidenceColor(r.confidence)}`}>
                    {Math.round(r.confidence * 100)}%
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Low confidence warning */}
      {top.confidence < 0.5 && (
        <div className="bg-amber-900/30 border border-amber-700/50 rounded-xl px-4 py-3 text-sm text-amber-300">
          ⚠️ Уверенность низкая. Попробуйте сфотографировать экспонат с другого угла или ближе.
        </div>
      )}

      {/* Actions */}
      <div className="flex gap-3">
        <button onClick={onRetry} className="btn-primary flex-1 flex items-center justify-center gap-2">
          <RotateCcw size={16} /> Ещё раз
        </button>
        <button onClick={onChangeMuseum} className="btn-secondary flex-1 flex items-center justify-center gap-2">
          <Home size={16} /> В меню
        </button>
      </div>
    </div>
  );
}

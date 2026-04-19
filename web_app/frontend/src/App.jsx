import { useState, useEffect } from "react";
import MuseumSelect from "./components/MuseumSelect";
import MuseumMap from "./components/MuseumMap";
import MuseumDetail from "./components/MuseumDetail";
import CameraView from "./components/CameraView";
import ExhibitResult from "./components/ExhibitResult";
import AdminPanel from "./components/AdminPanel";
import { fetchMuseums } from "./api";
import { Loader2, Settings } from "lucide-react";

export default function App() {
  const [step, setStep] = useState("select"); // select | map | detail | camera | result | admin
  const [museum, setMuseum] = useState(null);
  const [allMuseums, setAllMuseums] = useState([]);
  const [results, setResults] = useState(null);
  const [loadingUrl, setLoadingUrl] = useState(false);

  function handleMuseumSelected(m) {
    setMuseum(m);
    setStep("detail");
  }

  function handleStartCamera() {
    setStep("camera");
  }

  function handleResults(data) {
    setResults(data);
    setStep("result");
  }

  function handleReset() {
    setResults(null);
    setStep("camera");
  }

  function handleBackToDetail() {
    setResults(null);
    setStep("detail");
  }

  function handleChangeMuseum() {
    setMuseum(null);
    setResults(null);
    setStep("select");
  }

  function handleOpenMap(museums) {
    setAllMuseums(museums);
    setStep("map");
  }

  function handleAdminBack() {
    setStep("select");
  }

  // Чтение museum_id из URL при старте
  useEffect(() => {
    const url = new URL(window.location.href);
    const museumId = url.searchParams.get("museum_id");
    if (!museumId) return;
    const id = parseInt(museumId, 10);
    if (!id || isNaN(id)) return;

    setLoadingUrl(true);
    fetchMuseums()
      .then((list) => {
        const found = list.find((m) => m.id === id);
        if (found) {
          setMuseum(found);
          setStep("detail");
        }
      })
      .catch(() => {})
      .finally(() => setLoadingUrl(false));
  }, []);

  if (loadingUrl) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-stone-950 text-museum-400 gap-4">
        <Loader2 size={40} className="animate-spin" />
        <p>Загрузка музея...</p>
      </div>
    );
  }

  const showHeader = step !== "map" && step !== "detail";
  const showSteps = !["admin", "map", "detail"].includes(step);

  return (
    <div className="min-h-screen flex flex-col">
      {/* Header */}
      {showHeader && (
        <header className="bg-stone-900 border-b border-stone-800 px-4 py-3 flex items-center gap-3">
          <span className="text-2xl">🏛</span>
          <div className="flex-1 min-w-0">
            <h1 className="font-bold text-museum-400 text-lg leading-tight">
              Интерактивный музей
            </h1>
            {museum && (
              <p className="text-stone-400 text-sm truncate">{museum.name || museum.display_name}</p>
            )}
          </div>
          {museum && step !== "admin" && (
            <button
              onClick={handleChangeMuseum}
              className="text-xs text-stone-400 hover:text-stone-200 transition-colors shrink-0"
            >
              Сменить музей
            </button>
          )}
          {step === "select" && (
            <button
              onClick={() => setStep("admin")}
              className="p-1.5 text-stone-600 hover:text-stone-400 transition-colors shrink-0"
              title="Панель администратора"
            >
              <Settings size={17} />
            </button>
          )}
        </header>
      )}

      {/* Steps indicator */}
      {showSteps && (
        <div className="bg-stone-900 border-b border-stone-800 px-4 py-2 flex gap-2 text-xs">
          {["select", "camera", "result"].map((s, i) => {
            const labels = ["1. Музей", "2. Камера", "3. Результат"];
            const active = s === step;
            const done =
              (s === "select" && (step === "camera" || step === "result")) ||
              (s === "camera" && step === "result");
            return (
              <span
                key={s}
                className={`px-2 py-0.5 rounded-full font-medium ${
                  active
                    ? "bg-museum-500 text-white"
                    : done
                    ? "bg-stone-700 text-stone-300"
                    : "bg-stone-800 text-stone-500"
                }`}
              >
                {labels[i]}
              </span>
            );
          })}
        </div>
      )}

      {/* Main content */}
      <main className="flex-1 overflow-auto">
        {step === "select" && (
          <MuseumSelect onSelect={handleMuseumSelected} onOpenMap={handleOpenMap} />
        )}
        {step === "map" && (
          <MuseumMap
            museums={allMuseums}
            onSelect={handleMuseumSelected}
            onBack={() => setStep("select")}
          />
        )}
        {step === "detail" && museum && (
          <MuseumDetail
            museum={museum}
            onStartCamera={handleStartCamera}
            onBack={handleChangeMuseum}
          />
        )}
        {step === "camera" && museum && (
          <CameraView museum={museum} onResults={handleResults} />
        )}
        {step === "result" && results && (
          <ExhibitResult
            results={results}
            museum={museum}
            onRetry={handleReset}
            onChangeMuseum={handleBackToDetail}
          />
        )}
        {step === "admin" && (
          <AdminPanel onBack={handleAdminBack} />
        )}
      </main>
    </div>
  );
}

import { useRef, useEffect, useState, useCallback } from "react";
import { Camera, Loader2, AlertCircle, ZoomIn } from "lucide-react";
import { identifyExhibit } from "../api";

export default function CameraView({ museum, onResults, bridge }) {
  const videoRef = useRef(null);
  const canvasRef = useRef(null);
  const streamRef = useRef(null);
  const [camError, setCamError] = useState("");
  const [identifying, setIdentifying] = useState(false);
  const [hint, setHint] = useState("");
  const [facingMode, setFacingMode] = useState("environment"); // back camera by default
  const [videoDevices, setVideoDevices] = useState([]);
  const [selectedDeviceId, setSelectedDeviceId] = useState("");

  const stopCamera = useCallback(() => {
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }
  }, []);

  const syncVideoDevices = useCallback(async () => {
    if (!navigator.mediaDevices?.enumerateDevices) return;
    try {
      const devices = await navigator.mediaDevices.enumerateDevices();
      const cameras = devices.filter((d) => d.kind === "videoinput");
      setVideoDevices(cameras);
      if (!selectedDeviceId && cameras.length > 0) {
        const preferred = cameras.find((d) => /back|rear|environment|wide/i.test(d.label)) || cameras[0];
        if (preferred?.deviceId) {
          setSelectedDeviceId(preferred.deviceId);
        }
      }
    } catch {
    }
  }, [selectedDeviceId]);

  const startCamera = useCallback(async () => {
    setCamError("");
    setHint("");
    if (!navigator.mediaDevices?.getUserMedia) {
      setCamError("Камера недоступна в этом окружении. Откройте приложение по HTTPS или внутри MAX.");
      return;
    }

    stopCamera();

    try {
      const attempts = [];
      if (selectedDeviceId) {
        attempts.push({ video: { deviceId: { exact: selectedDeviceId }, width: { ideal: 1280 }, height: { ideal: 720 } }, audio: false });
      }
      attempts.push(
        { video: { facingMode: { ideal: facingMode }, width: { ideal: 1280 }, height: { ideal: 720 } }, audio: false },
        { video: { facingMode }, audio: false },
        { video: true, audio: false }
      );

      let stream = null;
      let lastError = null;
      for (const constraints of attempts) {
        try {
          stream = await navigator.mediaDevices.getUserMedia(constraints);
          break;
        } catch (err) {
          lastError = err;
        }
      }

      if (!stream) {
        throw lastError || new Error("Не удалось открыть камеру");
      }

      streamRef.current = stream;
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        await new Promise((resolve) => {
          const video = videoRef.current;
          if (!video) {
            resolve();
            return;
          }
          if (video.readyState >= 1) {
            resolve();
            return;
          }
          const onLoaded = () => {
            video.removeEventListener("loadedmetadata", onLoaded);
            resolve();
          };
          video.addEventListener("loadedmetadata", onLoaded);
        });
        await videoRef.current.play().catch(() => {});
      }
      await syncVideoDevices();
    } catch (e) {
      const message = e?.name === "NotAllowedError"
        ? "Нет доступа к камере. Разрешите использование камеры в MAX или браузере."
        : e?.name === "NotFoundError"
        ? "Камера не найдена на устройстве."
        : e?.name === "NotReadableError"
        ? "Камера уже используется другим приложением."
        : "Не удалось запустить камеру. Попробуйте ещё раз.";
      setCamError(message);
    }
  }, [facingMode, selectedDeviceId, stopCamera, syncVideoDevices]);

  useEffect(() => {
    if (bridge?.isMiniApp && bridge?.raw?.requestScreenMaxBrightness) {
      bridge.raw.requestScreenMaxBrightness().catch(() => {});
    }
    startCamera();
    return () => {
      if (bridge?.isMiniApp && bridge?.raw?.restoreScreenBrightness) {
        bridge.raw.restoreScreenBrightness().catch(() => {});
      }
      stopCamera();
    };
  }, [bridge, startCamera, stopCamera]);

  useEffect(() => {
    syncVideoDevices();
  }, [syncVideoDevices]);

  async function handleCapture() {
    if (!videoRef.current || !canvasRef.current || identifying) return;
    const video = videoRef.current;
    const canvas = canvasRef.current;
    canvas.width = video.videoWidth;
    canvas.height = video.videoHeight;
    canvas.getContext("2d").drawImage(video, 0, 0);
    setIdentifying(true);
    setHint("");
    canvas.toBlob(
      async (blob) => {
        try {
          const data = await identifyExhibit(museum.id, blob);
          onResults(data);
        } catch (e) {
          if (e.message.includes("Модель")) {
            setHint("Модель для этого музея ещё не обучена. Обратитесь к администратору.");
          } else {
            setHint(`Ошибка: ${e.message}`);
          }
        } finally {
          setIdentifying(false);
        }
      },
      "image/jpeg",
      0.9
    );
  }

  function toggleCamera() {
    if (videoDevices.length > 1) {
      const currentIndex = videoDevices.findIndex((d) => d.deviceId === selectedDeviceId);
      const nextIndex = currentIndex >= 0 ? (currentIndex + 1) % videoDevices.length : 0;
      setSelectedDeviceId(videoDevices[nextIndex]?.deviceId || "");
      return;
    }
    setSelectedDeviceId("");
    setFacingMode((f) => (f === "environment" ? "user" : "environment"));
  }

  return (
    <div className="flex flex-col h-[calc(100vh-120px)] sm:h-full">
      {/* Camera viewport - reduced height on mobile */}
      <div className="relative bg-black flex-1 min-h-0" style={{ maxHeight: "55vh" }}>
        {camError ? (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 text-center p-6">
            <AlertCircle size={48} className="text-red-400" />
            <p className="text-red-300 text-sm">{camError}</p>
            <button onClick={startCamera} className="btn-secondary text-sm">
              Повторить
            </button>
          </div>
        ) : (
          <>
            <video
              ref={videoRef}
              autoPlay
              playsInline
              muted
              className="w-full h-full object-cover"
            />
            {/* Viewfinder overlay */}
            <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
              <div className="w-56 h-56 relative">
                {/* Corner brackets */}
                {[
                  "top-0 left-0 border-t-2 border-l-2",
                  "top-0 right-0 border-t-2 border-r-2",
                  "bottom-0 left-0 border-b-2 border-l-2",
                  "bottom-0 right-0 border-b-2 border-r-2",
                ].map((cls, i) => (
                  <span
                    key={i}
                    className={`absolute w-6 h-6 border-museum-400 ${cls}`}
                  />
                ))}
              </div>
            </div>
            {identifying && (
              <div className="absolute inset-0 bg-black/60 flex flex-col items-center justify-center gap-3">
                <Loader2 size={40} className="animate-spin text-museum-400" />
                <p className="text-museum-300 font-medium">Определяем экспонат...</p>
              </div>
            )}
          </>
        )}
        <canvas ref={canvasRef} className="hidden" />
      </div>

      {/* Controls - always visible, not shrinkable */}
      <div className="p-4 space-y-3 bg-stone-950 flex-shrink-0 pb-6 sm:pb-4">
        {hint && (
          <div className="flex items-start gap-2 bg-stone-800 rounded-xl p-3 text-sm text-amber-300">
            <AlertCircle size={16} className="shrink-0 mt-0.5" />
            <span>{hint}</span>
          </div>
        )}

        <p className="text-center text-stone-400 text-sm">
          Наведите камеру на экспонат и нажмите кнопку
        </p>

        <div className="flex gap-3 items-center justify-center">
          {/* Toggle camera button */}
          <button
            onClick={toggleCamera}
            className="w-10 h-10 rounded-full bg-stone-800 flex items-center justify-center text-stone-400 hover:bg-stone-700 transition-colors"
            title="Переключить камеру"
          >
            <ZoomIn size={18} />
          </button>

          {/* Main capture button */}
          <button
            onClick={handleCapture}
            disabled={identifying || !!camError}
            className="w-16 h-16 rounded-full bg-museum-500 hover:bg-museum-400 disabled:opacity-50 flex items-center justify-center transition-colors shadow-lg shadow-museum-500/30"
          >
            {identifying ? (
              <Loader2 size={28} className="animate-spin text-white" />
            ) : (
              <Camera size={28} className="text-white" />
            )}
          </button>

          {/* Placeholder for symmetry */}
          <div className="w-10 h-10" />
        </div>
      </div>
    </div>
  );
}

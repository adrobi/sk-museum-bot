import { useState, useEffect, useCallback, useRef } from "react";
import {
  LogOut, RefreshCw, BrainCircuit, CheckCircle2, XCircle,
  Loader2, Mail, KeyRound, ChevronRight, ChevronLeft,
  Upload, Trash2, ImageOff, Play, Camera,
} from "lucide-react";
import {
  adminLogin, adminVerify, fetchAdminMuseums, adminLogout,
  fetchMuseumStatus, fetchAdminExhibits, uploadExhibitPhotos,
  listExhibitPhotos, deleteExhibitPhoto, startAdminTraining,
  getTrainingProgress,
} from "../api";

const SESSION_KEY = "museum_admin_token";
const ROLE_KEY    = "museum_admin_role";

export default function AdminPanel({ onBack, bridge }) {
  const [phase, setPhase] = useState("login");
  const [email, setEmail] = useState("");
  const [maxId, setMaxId] = useState("");
  const [code, setCode]   = useState("");
  const [token, setToken] = useState(() => sessionStorage.getItem(SESSION_KEY) || "");
  const [role, setRole]   = useState(() => sessionStorage.getItem(ROLE_KEY) || "");
  const [loginContext, setLoginContext] = useState(null);

  const [museums, setMuseums]             = useState([]);
  const [statuses, setStatuses]           = useState({});
  const [selectedMuseum, setSelectedMuseum] = useState(null);
  const [exhibits, setExhibits]           = useState([]);
  const [photos, setPhotos]               = useState({});   // {id: [{filename,url}]}

  const [expandedId, setExpandedId]   = useState(null);
  const [uploading, setUploading]     = useState({});       // {exhibitId: bool}
  const [training, setTraining]       = useState(null);     // {status,percent,message}
  const [loading, setLoading]         = useState(false);
  const [error, setError]             = useState("");
  const [info, setInfo]               = useState("");

  const pollingRef = useRef(null);

  // ── helpers ────────────────────────────────────────────────────────────────
  function saveSession(tok, r) {
    sessionStorage.setItem(SESSION_KEY, tok);
    sessionStorage.setItem(ROLE_KEY, r);
    setToken(tok); setRole(r);
  }
  function clearSession() {
    sessionStorage.removeItem(SESSION_KEY);
    sessionStorage.removeItem(ROLE_KEY);
    setToken(""); setRole("");
  }
  function stopPolling() {
    if (pollingRef.current) { clearInterval(pollingRef.current); pollingRef.current = null; }
  }

  useEffect(() => () => stopPolling(), []);

  useEffect(() => {
    if (token) return;
    if (!bridge?.isMiniApp || !bridge?.initData || phase !== "login") return;

    let cancelled = false;
    async function startMaxLogin() {
      setLoading(true);
      setError("");
      setInfo("Проверяем MAX-профиль и отправляем код на почту...");
      try {
        const data = await adminLogin({ maxInitData: bridge.initData });
        if (cancelled) return;
        setLoginContext({ email: data.email, userId: data.user_id, authSource: data.auth_source });
        setInfo(`Код отправлен на ${data.masked_email}`);
        setPhase("otp");
      } catch (err) {
        if (cancelled) return;
        setInfo("");
        setError(err.message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    startMaxLogin();
    return () => { cancelled = true; };
  }, [bridge, token, phase]);

  // ── load museums ───────────────────────────────────────────────────────────
  const loadMuseums = useCallback(async (tok) => {
    setLoading(true); setError("");
    try {
      const list = await fetchAdminMuseums(tok);
      setMuseums(list);
      const st = {};
      await Promise.all(list.map(async (m) => {
        try { st[m.id] = await fetchMuseumStatus(m.id); }
        catch { st[m.id] = { model_ready: false, num_classes: 0 }; }
      }));
      setStatuses(st);
      setPhase("museums");
    } catch (e) {
      if (e.message.includes("401")) { clearSession(); setPhase("login"); }
      setError(e.message);
    } finally { setLoading(false); }
  }, []);

  useEffect(() => { if (token) loadMuseums(token); }, []);

  // ── load exhibits ──────────────────────────────────────────────────────────
  async function openMuseum(museum) {
    setLoading(true); setError("");
    setExhibits([]); setPhotos({}); setExpandedId(null);
    stopPolling(); setTraining(null);
    try {
      const list = await fetchAdminExhibits(museum.id, token);
      setExhibits(list);
      const ph = {};
      await Promise.all(list.map(async (ex) => {
        try { ph[ex.id] = await listExhibitPhotos(ex.id, token); }
        catch { ph[ex.id] = []; }
      }));
      setPhotos(ph);
      setSelectedMuseum(museum);
      setPhase("exhibits");
    } catch (e) { setError(e.message); }
    finally { setLoading(false); }
  }

  // ── photo ops ──────────────────────────────────────────────────────────────
  async function handleUpload(exhibitId, files) {
    if (!files?.length) return;
    const current = photos[exhibitId]?.length ?? 0;
    if (current + files.length > 20) {
      setError(`Максимум 20 фото на экспонат (уже: ${current})`); return;
    }
    setUploading(u => ({ ...u, [exhibitId]: true })); setError("");
    try {
      await uploadExhibitPhotos(exhibitId, files, token);
      const updated = await listExhibitPhotos(exhibitId, token);
      setPhotos(p => ({ ...p, [exhibitId]: updated }));
    } catch (e) { setError(e.message); }
    finally { setUploading(u => ({ ...u, [exhibitId]: false })); }
  }

  async function handleDelete(exhibitId, filename) {
    try {
      await deleteExhibitPhoto(exhibitId, filename, token);
      const updated = await listExhibitPhotos(exhibitId, token);
      setPhotos(p => ({ ...p, [exhibitId]: updated }));
    } catch (e) { setError(e.message); }
  }

  // ── training ───────────────────────────────────────────────────────────────
  const exhibitsWithPhotos = Object.values(photos).filter(arr => arr?.length > 0).length;

  function startPolling(museumId) {
    stopPolling();
    pollingRef.current = setInterval(async () => {
      try {
        const prog = await getTrainingProgress(museumId, token);
        setTraining(prog);
        if (prog.status === "done" || prog.status === "error") {
          stopPolling();
          if (prog.status === "done") {
            const st = await fetchMuseumStatus(museumId).catch(() => null);
            if (st) setStatuses(s => ({ ...s, [museumId]: st }));
          }
        }
      } catch {}
    }, 2000);
  }

  async function handleStartTraining() {
    if (exhibitsWithPhotos < 2) {
      setError("Нужно загрузить фото хотя бы для 2 экспонатов"); return;
    }
    setError(""); setInfo("");
    setTraining({ status: "training", percent: 0, message: "Запуск..." });
    try {
      await startAdminTraining(selectedMuseum.id, token);
      startPolling(selectedMuseum.id);
    } catch (e) {
      setTraining({ status: "error", percent: 0, message: e.message });
    }
  }

  // ── auth ───────────────────────────────────────────────────────────────────
  async function handleSendCode(e) {
    e.preventDefault(); setLoading(true); setError("");
    try {
      const normalizedEmail = email.trim();
      const normalizedMaxId = maxId.trim();
      if (!normalizedEmail || !normalizedMaxId) {
        throw new Error("Укажите email и MAX ID");
      }
      const data = await adminLogin({ email: normalizedEmail, identifier: normalizedMaxId });
      setLoginContext({ email: normalizedEmail, identifier: normalizedMaxId, authSource: data.auth_source });
      setInfo(`Код отправлен на ${data.masked_email}`);
      setPhase("otp");
    }
    catch (err) { setError(err.message); }
    finally { setLoading(false); }
  }

  async function handleVerify(e) {
    e.preventDefault(); setLoading(true); setError("");
    try {
      const useMaxSessionId = loginContext?.authSource === "max";
      const data = await adminVerify({
        email: loginContext?.email,
        identifier: loginContext?.identifier,
        userId: useMaxSessionId ? loginContext?.userId : undefined,
        code,
      });
      saveSession(data.token, data.role);
      loadMuseums(data.token);
    } catch (err) { setError(err.message); }
    finally { setLoading(false); }
  }

  async function handleLogout() {
    stopPolling();
    await adminLogout(token);
    clearSession();
    setMuseums([]); setStatuses({}); setSelectedMuseum(null); setExhibits([]);
    setPhase("login");
  }

  // ── render ─────────────────────────────────────────────────────────────────
  if (phase === "login") return (
    <div className="p-4 max-w-sm mx-auto space-y-6 pt-8">
      <div className="text-center space-y-1">
        <div className="text-4xl mb-2">🛡</div>
        <h2 className="text-xl font-bold text-stone-100">Вход для администраторов</h2>
        <p className="text-stone-500 text-sm">
          {bridge?.isMiniApp
            ? "Если ваш MAX ID зарегистрирован, код придёт на привязанную почту"
            : "В браузере укажите рабочий email и ваш MAX ID"}
        </p>
      </div>
      {!bridge?.isMiniApp && (
        <form onSubmit={handleSendCode} className="space-y-3">
          <div className="relative">
            <Mail size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-stone-500" />
            <input type="email" required value={email} onChange={e => setEmail(e.target.value)}
              placeholder="admin@example.com"
              className="w-full bg-stone-800 border border-stone-700 rounded-xl pl-9 pr-4 py-2.5 text-sm text-stone-100 placeholder-stone-500 focus:outline-none focus:border-museum-500 transition-colors" />
          </div>
          <div className="relative">
            <KeyRound size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-stone-500" />
            <input type="text" inputMode="numeric" required value={maxId} onChange={e => setMaxId(e.target.value.replace(/\D/g, ""))}
              placeholder="MAX ID"
              className="w-full bg-stone-800 border border-stone-700 rounded-xl pl-9 pr-4 py-2.5 text-sm text-stone-100 placeholder-stone-500 focus:outline-none focus:border-museum-500 transition-colors" />
          </div>
          {error && <p className="text-red-400 text-sm">{error}</p>}
          {info && <p className="text-green-400 text-sm">{info}</p>}
          <button type="submit" disabled={loading}
            className="btn-primary w-full flex items-center justify-center gap-2">
            {loading ? <Loader2 size={16} className="animate-spin" /> : <ChevronRight size={16} />}
            Получить код
          </button>
        </form>
      )}
      {bridge?.isMiniApp && (
        <div className="space-y-3">
          <div className="bg-stone-900/70 border border-stone-800 rounded-xl px-4 py-3 text-sm text-stone-300">
            <p>MAX пользователь: <span className="text-stone-100">{bridge?.user?.first_name || bridge?.user?.username || bridge?.user?.id}</span></p>
            <p className="text-stone-500 mt-1">ID: {bridge?.user?.id ?? "—"}</p>
          </div>
          {loading && <p className="text-stone-400 text-sm text-center">Отправляем код на почту...</p>}
          {error && <p className="text-red-400 text-sm text-center">{error}</p>}
          {info && <p className="text-green-400 text-sm text-center">{info}</p>}
        </div>
      )}
      <button onClick={onBack} className="w-full text-sm text-stone-500 hover:text-stone-300 transition-colors">← Назад</button>
    </div>
  );

  if (phase === "otp") return (
    <div className="p-4 max-w-sm mx-auto space-y-6 pt-8">
      <div className="text-center space-y-1">
        <div className="text-4xl mb-2">📩</div>
        <h2 className="text-xl font-bold text-stone-100">Введите код</h2>
        <p className="text-stone-500 text-sm">Код отправлен на <span className="text-stone-300">{loginContext?.email || "вашу почту"}</span></p>
      </div>
      <form onSubmit={handleVerify} className="space-y-3">
        <div className="relative">
          <KeyRound size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-stone-500" />
          <input type="text" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} required
            value={code} onChange={e => setCode(e.target.value.replace(/\D/g, ""))}
            placeholder="6 цифр"
            className="w-full bg-stone-800 border border-stone-700 rounded-xl pl-9 pr-4 py-2.5 text-sm text-stone-100 placeholder-stone-500 focus:outline-none focus:border-museum-500 transition-colors tracking-widest text-center text-lg" />
        </div>
        {error && <p className="text-red-400 text-sm text-center">{error}</p>}
        <button type="submit" disabled={loading || code.length !== 6}
          className="btn-primary w-full flex items-center justify-center gap-2">
          {loading ? <Loader2 size={16} className="animate-spin" /> : <KeyRound size={16} />}
          Войти
        </button>
      </form>
      <button onClick={() => { setPhase("login"); setError(""); setInfo(""); setCode(""); setLoginContext(null); }}
        className="w-full text-sm text-stone-500 hover:text-stone-300 transition-colors">← Назад</button>
    </div>
  );

  if (phase === "museums") return (
    <div className="p-4 max-w-lg mx-auto space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="font-bold text-stone-100">🛡 Панель управления</h2>
          <p className="text-stone-500 text-xs mt-0.5">{role === "bot_admin" ? "Главный администратор" : "Администратор музея"}</p>
        </div>
        <div className="flex gap-2">
          <button onClick={() => loadMuseums(token)} disabled={loading}
            className="p-2 text-stone-400 hover:text-stone-100 transition-colors" title="Обновить">
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
          <button onClick={handleLogout} className="p-2 text-stone-400 hover:text-red-400 transition-colors" title="Выйти">
            <LogOut size={16} />
          </button>
        </div>
      </div>

      {error && <div className="bg-red-900/30 border border-red-700/50 rounded-xl px-4 py-2.5 text-sm text-red-300">❌ {error}</div>}

      {loading && museums.length === 0
        ? <div className="flex justify-center py-12"><Loader2 size={32} className="animate-spin text-museum-400" /></div>
        : museums.map(m => {
          const st = statuses[m.id];
          return (
            <div key={m.id} className="card p-4 space-y-3">
              <div className="flex items-center gap-3">
                {m.image_url
                  ? <img src={m.image_url} alt="" className="w-12 h-12 rounded-lg object-cover shrink-0 bg-stone-800"
                      onError={e => { e.currentTarget.style.display = "none"; e.currentTarget.nextSibling.style.display = "flex"; }} />
                  : null}
                <div className="w-12 h-12 rounded-lg bg-stone-800 items-center justify-center shrink-0 text-xl"
                  style={{ display: m.image_url ? "none" : "flex" }}>🏛</div>
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-stone-100 text-sm">{m.name}</p>
                  <p className="text-xs text-stone-500 mt-0.5">
                    {st?.model_ready
                      ? <span className="text-green-400">✅ Модель обучена ({st.num_classes} кл.)</span>
                      : <span className="text-amber-500">⚠️ Не обучена</span>}
                  </p>
                </div>
                <button onClick={() => openMuseum(m)}
                  className="flex items-center gap-1.5 px-3 py-1.5 bg-museum-500/20 border border-museum-500/50 text-museum-300 rounded-lg text-xs font-medium hover:bg-museum-500/30 transition-colors shrink-0">
                  <BrainCircuit size={13} /> Обучить
                </button>
              </div>
            </div>
          );
        })}
      <button onClick={onBack} className="w-full text-sm text-stone-500 hover:text-stone-300 transition-colors pt-2">← На главную</button>
    </div>
  );

  if (phase === "exhibits") return (
    <div className="p-4 max-w-2xl mx-auto space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between gap-2">
        <button onClick={() => { stopPolling(); setPhase("museums"); setSelectedMuseum(null); }}
          className="flex items-center gap-1 text-stone-400 hover:text-stone-100 transition-colors text-sm">
          <ChevronLeft size={16} /> Музеи
        </button>
        <div className="flex-1 min-w-0 text-center">
          <p className="font-semibold text-stone-100 text-sm truncate">{selectedMuseum?.name}</p>
        </div>
        <button onClick={handleLogout} className="p-1.5 text-stone-500 hover:text-red-400 transition-colors">
          <LogOut size={15} />
        </button>
      </div>

      {/* Training progress banner */}
      {training && (
        <div className={`rounded-xl p-3 border space-y-2 ${
          training.status === "done"    ? "bg-green-900/30 border-green-700/50" :
          training.status === "error"   ? "bg-red-900/30 border-red-700/50" :
                                          "bg-stone-800/60 border-stone-700"
        }`}>
          <div className="flex items-center justify-between text-sm">
            <span className={training.status === "done" ? "text-green-300" : training.status === "error" ? "text-red-300" : "text-stone-300"}>
              {training.status === "training" && <Loader2 size={14} className="inline animate-spin mr-1.5" />}
              {training.message}
            </span>
            <span className="text-stone-400 text-xs">{training.percent}%</span>
          </div>
          {training.status === "training" && (
            <div className="h-1.5 bg-stone-700 rounded-full overflow-hidden">
              <div className="h-full bg-museum-500 rounded-full transition-all duration-500"
                style={{ width: `${training.percent}%` }} />
            </div>
          )}
        </div>
      )}

      {error && <div className="bg-red-900/30 border border-red-700/50 rounded-xl px-4 py-2.5 text-sm text-red-300">❌ {error}</div>}
      {info  && <div className="bg-green-900/30 border border-green-700/50 rounded-xl px-4 py-2.5 text-sm text-green-300">✅ {info}</div>}

      {/* Start training button */}
      <button onClick={handleStartTraining}
        disabled={training?.status === "training" || exhibitsWithPhotos < 2}
        className={`w-full flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold transition-colors
          ${training?.status === "training"
            ? "bg-stone-800 border border-stone-700 text-stone-500 cursor-not-allowed"
            : exhibitsWithPhotos >= 2
              ? "bg-museum-500 text-white hover:bg-museum-400"
              : "bg-stone-800 border border-stone-700 text-stone-600 cursor-not-allowed"}`}>
        {training?.status === "training"
          ? <><Loader2 size={16} className="animate-spin" /> Обучение в процессе...</>
          : <><Play size={16} /> Начать обучение ({exhibitsWithPhotos} из {exhibits.length} экспонатов с фото)</>}
      </button>
      {exhibitsWithPhotos < 2 && training?.status !== "training" && (
        <p className="text-center text-amber-500/80 text-xs -mt-2">Загрузите фото хотя бы для 2 экспонатов</p>
      )}

      {/* Exhibit list */}
      {loading
        ? <div className="flex justify-center py-10"><Loader2 size={28} className="animate-spin text-museum-400" /></div>
        : exhibits.length === 0
          ? <p className="text-center text-stone-500 py-8">Нет экспонатов</p>
          : exhibits.map(ex => {
            const exPhotos = photos[ex.id] ?? [];
            const isExpanded = expandedId === ex.id;
            const isUploading = uploading[ex.id];
            const count = exPhotos.length;

            return (
              <div key={ex.id} className="card overflow-hidden">
                {/* Exhibit header */}
                <button
                  onClick={() => setExpandedId(isExpanded ? null : ex.id)}
                  className="w-full p-3 flex items-center gap-3 text-left hover:bg-stone-800/40 transition-colors">
                  {ex.image_url
                    ? <img src={ex.image_url} alt="" className="w-10 h-10 rounded-lg object-cover shrink-0 bg-stone-800"
                        onError={e => { e.currentTarget.style.display = "none"; e.currentTarget.nextSibling.style.display = "flex"; }} />
                    : null}
                  <div className="w-10 h-10 rounded-lg bg-stone-800 items-center justify-center shrink-0 text-lg"
                    style={{ display: ex.image_url ? "none" : "flex" }}><ImageOff size={18} className="text-stone-600" /></div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium text-stone-100 truncate">{ex.title}</p>
                    <p className={`text-xs mt-0.5 ${count >= 2 ? "text-green-400" : count === 1 ? "text-amber-400" : "text-stone-500"}`}>
                      <Camera size={11} className="inline mr-1" />
                      {count === 0 ? "Нет фото для обучения" : `${count}/20 фото`}
                    </p>
                  </div>
                  <ChevronRight size={16} className={`text-stone-500 transition-transform ${isExpanded ? "rotate-90" : ""}`} />
                </button>

                {/* Expanded: photo strip + upload */}
                {isExpanded && (
                  <div className="px-3 pb-3 space-y-2 border-t border-stone-700/50 pt-3">
                    {/* Thumbnails */}
                    {exPhotos.length > 0 && (
                      <div className="flex flex-wrap gap-2">
                        {exPhotos.map(ph => (
                          <div key={ph.filename} className="relative group w-16 h-16">
                            <img src={ph.url} alt="" className="w-full h-full object-cover rounded-lg bg-stone-800" />
                            <button
                              onClick={() => handleDelete(ex.id, ph.filename)}
                              className="absolute -top-1.5 -right-1.5 w-5 h-5 bg-red-600 rounded-full items-center justify-center hidden group-hover:flex hover:bg-red-500 transition-colors">
                              <Trash2 size={10} className="text-white" />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}

                    {/* Upload button */}
                    {count < 20 && (
                      <label className={`flex items-center justify-center gap-2 py-2 rounded-xl border border-dashed text-sm cursor-pointer transition-colors
                        ${isUploading
                          ? "border-stone-700 text-stone-600 cursor-not-allowed"
                          : "border-stone-600 text-stone-400 hover:border-museum-500 hover:text-museum-300"}`}>
                        {isUploading
                          ? <><Loader2 size={14} className="animate-spin" /> Загрузка...</>
                          : <><Upload size={14} /> Добавить фото ({20 - count} осталось)</>}
                        <input type="file" accept="image/*" multiple className="hidden" disabled={isUploading}
                          onChange={e => handleUpload(ex.id, Array.from(e.target.files || []))} />
                      </label>
                    )}
                    {count >= 20 && (
                      <p className="text-center text-xs text-stone-500">Максимум фото загружено (20/20)</p>
                    )}
                  </div>
                )}
              </div>
            );
          })}
    </div>
  );

  return null;
}

const BASE = "/api";

// ── Admin ────────────────────────────────────────────────────────────────────

export async function adminLogin(email) {
  const r = await fetch(`${BASE}/admin/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  });
  if (!r.ok) {
    const e = await r.json().catch(() => ({}));
    throw new Error(e.detail || "Ошибка отправки кода");
  }
  return r.json();
}

export async function adminVerify(email, code) {
  const r = await fetch(`${BASE}/admin/verify`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, code }),
  });
  if (!r.ok) {
    const e = await r.json().catch(() => ({}));
    throw new Error(e.detail || "Неверный код");
  }
  return r.json();
}

export async function fetchAdminMuseums(token) {
  const r = await fetch(`${BASE}/admin/museums`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok) {
    const e = await r.json().catch(() => ({}));
    throw new Error(e.detail || "Ошибка загрузки музеев");
  }
  return r.json();
}

export async function adminLogout(token) {
  await fetch(`${BASE}/admin/logout`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  }).catch(() => {});
}

export async function fetchAdminExhibits(museumId, token) {
  const r = await fetch(`${BASE}/admin/museums/${museumId}/exhibits`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.detail || "Ошибка загрузки экспонатов"); }
  return r.json();
}

export async function listExhibitPhotos(exhibitId, token) {
  const r = await fetch(`${BASE}/admin/exhibits/${exhibitId}/photos`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok) return [];
  return r.json();
}

export async function uploadExhibitPhotos(exhibitId, files, token) {
  const fd = new FormData();
  for (const f of files) fd.append("files", f);
  const r = await fetch(`${BASE}/admin/exhibits/${exhibitId}/photos`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
    body: fd,
  });
  if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.detail || "Ошибка загрузки фото"); }
  return r.json();
}

export async function deleteExhibitPhoto(exhibitId, filename, token) {
  await fetch(`${BASE}/admin/exhibits/${exhibitId}/photos/${encodeURIComponent(filename)}`, {
    method: "DELETE",
    headers: { Authorization: `Bearer ${token}` },
  });
}

export async function startAdminTraining(museumId, token) {
  const r = await fetch(`${BASE}/admin/train/${museumId}`, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.detail || "Ошибка запуска обучения"); }
  return r.json();
}

export async function getTrainingProgress(museumId, token) {
  const r = await fetch(`${BASE}/admin/train/${museumId}/progress`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!r.ok) return { status: "idle", percent: 0, message: "" };
  return r.json();
}

export async function fetchMuseums() {
  const r = await fetch(`${BASE}/museums/`);
  if (!r.ok) throw new Error("Ошибка загрузки музеев");
  return r.json();
}

export async function fetchNearbyMuseums(lat, lon) {
  const r = await fetch(`${BASE}/museums/nearby?lat=${lat}&lon=${lon}`);
  if (!r.ok) throw new Error("Ошибка геолокации");
  return r.json();
}

export async function fetchMuseumDetail(museumId) {
  const r = await fetch(`${BASE}/museums/${museumId}`);
  if (!r.ok) throw new Error("Ошибка загрузки музея");
  return r.json();
}

export async function fetchMuseumExhibitions(museumId) {
  const r = await fetch(`${BASE}/museums/${museumId}/exhibitions`);
  if (!r.ok) throw new Error("Ошибка загрузки выставок");
  return r.json();
}

export async function fetchMuseumExhibits(museumId) {
  const r = await fetch(`${BASE}/museums/${museumId}/exhibits`);
  if (!r.ok) throw new Error("Ошибка загрузки экспонатов");
  return r.json();
}

export async function fetchMuseumStatus(museumId) {
  const r = await fetch(`${BASE}/museums/${museumId}/model-status`);
  if (!r.ok) throw new Error("Ошибка статуса");
  return r.json();
}

export async function identifyExhibit(museumId, imageBlob) {
  const form = new FormData();
  form.append("museum_id", String(museumId));
  form.append("image", imageBlob, "capture.jpg");
  const r = await fetch(`${BASE}/recognition/identify`, {
    method: "POST",
    body: form,
  });
  if (!r.ok) {
    const err = await r.json().catch(() => ({}));
    throw new Error(err.detail || "Ошибка распознавания");
  }
  return r.json();
}

export async function triggerTraining(museumId) {
  const r = await fetch(`${BASE}/recognition/train/${museumId}`, {
    method: "POST",
  });
  if (!r.ok) {
    const err = await r.json().catch(() => ({}));
    throw new Error(err.detail || "Ошибка запуска обучения");
  }
  return r.json();
}

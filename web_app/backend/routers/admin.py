import os
import random
import secrets
import smtplib
import logging
import uuid
from datetime import datetime, timedelta
from email.mime.text import MIMEText
from pathlib import Path
from typing import List

from fastapi import APIRouter, HTTPException, Depends, BackgroundTasks, UploadFile, File
from fastapi.responses import FileResponse
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
from pydantic import BaseModel

from db import database
from utils import img_url

UPLOAD_DIR = Path("/tmp/training_uploads")
UPLOAD_DIR.mkdir(parents=True, exist_ok=True)

_training_progress: dict[int, dict] = {}

router = APIRouter()
security = HTTPBearer(auto_error=False)

_sessions: dict[str, dict] = {}


class LoginRequest(BaseModel):
    email: str


class VerifyRequest(BaseModel):
    email: str
    code: str


def _send_otp_email(to: str, code: str):
    from_addr = os.getenv("SMTP_USER", "")
    password = os.getenv("SMTP_PASS", "")
    host = os.getenv("SMTP_HOST", "smtp.gmail.com")
    port = int(os.getenv("SMTP_PORT", "587"))
    if not from_addr or not password:
        logging.warning("SMTP не настроен, код: %s", code)
        return
    msg = MIMEText(
        f"Ваш код входа в панель управления музеями:\n\n"
        f"  {code}\n\n"
        f"Код действует 5 минут. Никому не сообщайте.",
        "plain",
        "utf-8",
    )
    msg["Subject"] = "Код входа — Музеи СК"
    msg["From"] = from_addr
    msg["To"] = to
    try:
        with smtplib.SMTP(host, port, timeout=10) as server:
            server.starttls()
            server.login(from_addr, password)
            server.sendmail(from_addr, to, msg.as_string())
        logging.info("OTP отправлен на %s", to)
    except Exception as exc:
        logging.error("SMTP ошибка: %s", exc)


def get_session(creds: HTTPAuthorizationCredentials = Depends(security)) -> dict:
    if not creds:
        raise HTTPException(status_code=401, detail="Требуется авторизация")
    session = _sessions.get(creds.credentials)
    if not session:
        raise HTTPException(status_code=401, detail="Недействительный токен")
    if datetime.now() > session["expires_at"]:
        _sessions.pop(creds.credentials, None)
        raise HTTPException(status_code=401, detail="Сессия истекла, войдите снова")
    return session


@router.post("/login")
async def admin_login(req: LoginRequest):
    row = await database.fetch_one(
        "SELECT user_id, role FROM staff WHERE email=:email AND is_active=true",
        {"email": req.email},
    )
    if not row:
        raise HTTPException(status_code=404, detail="Email не найден в системе")
    if row["role"] not in ("bot_admin", "museum_admin"):
        raise HTTPException(status_code=403, detail="Недостаточно прав для доступа")

    code = str(random.randint(100000, 999999))
    expires_at = datetime.now() + timedelta(minutes=5)
    await database.execute(
        "UPDATE staff SET current_otp=:otp, otp_expires_at=:exp WHERE email=:email",
        {"otp": code, "exp": expires_at, "email": req.email},
    )
    _send_otp_email(req.email, code)
    return {"message": "Код отправлен на почту"}


@router.post("/verify")
async def admin_verify(req: VerifyRequest):
    row = await database.fetch_one(
        "SELECT user_id, role, current_otp, otp_expires_at FROM staff WHERE email=:email",
        {"email": req.email},
    )
    if not row:
        raise HTTPException(status_code=404, detail="Не найдено")
    if row["current_otp"] != req.code:
        raise HTTPException(status_code=400, detail="Неверный код")
    if row["otp_expires_at"] is None or datetime.now() > row["otp_expires_at"]:
        raise HTTPException(status_code=400, detail="Код истёк")

    await database.execute(
        "UPDATE staff SET current_otp=NULL, last_login=NOW() WHERE email=:email",
        {"email": req.email},
    )
    token = secrets.token_urlsafe(32)
    _sessions[token] = {
        "user_id": row["user_id"],
        "role": row["role"],
        "expires_at": datetime.now() + timedelta(hours=24),
    }
    return {"token": token, "role": row["role"]}


@router.get("/museums")
async def admin_museums(session: dict = Depends(get_session)):
    if session["role"] == "bot_admin":
        rows = await database.fetch_all(
            """
            SELECT m.id,
                   COALESCE(NULLIF(m.short_name,''), m.name) AS name,
                   m.image_url,
                   (SELECT COUNT(*) FROM exhibits e
                    JOIN exhibitions ex ON e.exhibition_id = ex.id
                    WHERE ex.museum_id = m.id
                      AND e.is_active = true
                      AND e.image_url IS NOT NULL) AS exhibit_photo_count
            FROM museums m ORDER BY m.name
            """
        )
    else:
        rows = await database.fetch_all(
            """
            SELECT m.id,
                   COALESCE(NULLIF(m.short_name,''), m.name) AS name,
                   m.image_url,
                   (SELECT COUNT(*) FROM exhibits e
                    JOIN exhibitions ex ON e.exhibition_id = ex.id
                    WHERE ex.museum_id = m.id
                      AND e.is_active = true
                      AND e.image_url IS NOT NULL) AS exhibit_photo_count
            FROM museums m
            JOIN staff s ON m.id = s.museum_id
            WHERE s.user_id = :uid ORDER BY m.name
            """,
            {"uid": session["user_id"]},
        )
    result = []
    for r in rows:
        d = dict(r)
        d["image_url"] = img_url(d.get("image_url"))
        result.append(d)
    return result


# ── Exhibits ────────────────────────────────────────────────────────────────


@router.get("/museums/{museum_id}/exhibits")
async def admin_exhibits(museum_id: int, session: dict = Depends(get_session)):
    rows = await database.fetch_all(
        """
        SELECT e.id, e.title, COALESCE(e.description, '') AS description, e.image_url
        FROM exhibits e
        JOIN exhibitions ex ON e.exhibition_id = ex.id
        WHERE ex.museum_id = :mid AND e.is_active = true
        ORDER BY ex.title, e.title
        """,
        {"mid": museum_id},
    )
    result = []
    for r in rows:
        d = dict(r)
        d["image_url"] = img_url(d.get("image_url"))
        exhibit_dir = UPLOAD_DIR / str(r["id"])
        d["uploaded_count"] = len(list(exhibit_dir.iterdir())) if exhibit_dir.exists() else 0
        result.append(d)
    return result


# ── Photo upload / serve ─────────────────────────────────────────────────────


@router.post("/exhibits/{exhibit_id}/photos")
async def upload_exhibit_photos(
    exhibit_id: int,
    files: List[UploadFile] = File(...),
    session: dict = Depends(get_session),
):
    exhibit_dir = UPLOAD_DIR / str(exhibit_id)
    exhibit_dir.mkdir(parents=True, exist_ok=True)
    existing = list(exhibit_dir.iterdir())
    if len(existing) + len(files) > 20:
        raise HTTPException(
            status_code=400,
            detail=f"Максимум 20 фото на экспонат (уже загружено: {len(existing)})",
        )
    saved = []
    for f in files:
        if not f.content_type or not f.content_type.startswith("image/"):
            continue
        ext = Path(f.filename or "").suffix.lower() or ".jpg"
        filename = f"{uuid.uuid4().hex}{ext}"
        content = await f.read()
        (exhibit_dir / filename).write_bytes(content)
        saved.append({"filename": filename})
    total = len(list(exhibit_dir.iterdir()))
    return {"saved": len(saved), "total": total}


@router.get("/exhibits/{exhibit_id}/photos")
async def list_exhibit_photos(exhibit_id: int, session: dict = Depends(get_session)):
    exhibit_dir = UPLOAD_DIR / str(exhibit_id)
    if not exhibit_dir.exists():
        return []
    photos = [
        {"filename": f.name, "url": f"/api/admin/exhibits/{exhibit_id}/photos/{f.name}"}
        for f in sorted(exhibit_dir.iterdir())
    ]
    return photos


@router.get("/exhibits/{exhibit_id}/photos/{filename}")
async def serve_exhibit_photo(exhibit_id: int, filename: str):
    if "/" in filename or ".." in filename:
        raise HTTPException(status_code=400, detail="Invalid filename")
    path = UPLOAD_DIR / str(exhibit_id) / filename
    if not path.exists():
        raise HTTPException(status_code=404, detail="Not found")
    return FileResponse(path)


@router.delete("/exhibits/{exhibit_id}/photos/{filename}")
async def delete_exhibit_photo(
    exhibit_id: int, filename: str, session: dict = Depends(get_session)
):
    if "/" in filename or ".." in filename:
        raise HTTPException(status_code=400, detail="Invalid filename")
    path = UPLOAD_DIR / str(exhibit_id) / filename
    if path.exists():
        path.unlink()
    return {"ok": True}


# ── Training ─────────────────────────────────────────────────────────────────


async def _do_training(museum_id: int):
    """Background task: runs model training and updates _training_progress."""
    import asyncio
    from ml.train import train_museum_model

    _training_progress[museum_id] = {"status": "training", "percent": 0, "message": "Инициализация..."}

    # Fetch exhibit data HERE in the async context while DB is already connected,
    # so the thread doesn't need to re-connect the shared database object.
    rows = await database.fetch_all(
        """
        SELECT e.id AS exhibit_id, e.title, e.image_url
        FROM exhibits e
        JOIN exhibitions ex ON e.exhibition_id = ex.id
        WHERE ex.museum_id = :mid AND e.is_active = true
        ORDER BY e.id
        """,
        {"mid": museum_id},
    )
    exhibits_data = [dict(r) for r in rows]

    def cb(percent: int, message: str):
        _training_progress[museum_id] = {
            "status": "training",
            "percent": percent,
            "message": message,
        }

    try:
        loop = asyncio.get_event_loop()
        success = await loop.run_in_executor(
            None,
            lambda: train_museum_model(
                museum_id,
                upload_dir=UPLOAD_DIR,
                progress_cb=cb,
                exhibits=exhibits_data,
            ),
        )
        if success:
            _training_progress[museum_id] = {
                "status": "done",
                "percent": 100,
                "message": "✅ Обучение завершено успешно!",
            }
        else:
            _training_progress[museum_id] = {
                "status": "error",
                "percent": 0,
                "message": "❌ Недостаточно данных для обучения.",
            }
    except Exception as exc:
        _training_progress[museum_id] = {
            "status": "error",
            "percent": 0,
            "message": f"❌ Ошибка: {exc}",
        }


@router.post("/train/{museum_id}")
async def admin_train(
    museum_id: int,
    background_tasks: BackgroundTasks,
    session: dict = Depends(get_session),
):
    prog = _training_progress.get(museum_id, {})
    if prog.get("status") == "training":
        raise HTTPException(status_code=400, detail="Обучение уже запущено")
    row = await database.fetch_one("SELECT id FROM museums WHERE id=:id", {"id": museum_id})
    if not row:
        raise HTTPException(status_code=404, detail="Музей не найден")
    background_tasks.add_task(_do_training, museum_id)
    return {"status": "started", "museum_id": museum_id}


@router.get("/train/{museum_id}/progress")
async def training_progress_endpoint(museum_id: int, session: dict = Depends(get_session)):
    return _training_progress.get(
        museum_id, {"status": "idle", "percent": 0, "message": ""}
    )


@router.post("/logout")
async def admin_logout(creds: HTTPAuthorizationCredentials = Depends(security)):
    if creds:
        _sessions.pop(creds.credentials, None)
    return {"message": "Выход выполнен"}

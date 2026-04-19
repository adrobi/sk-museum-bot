from fastapi import APIRouter, UploadFile, File, HTTPException, Form
from PIL import Image
import io

from db import database
from ml.model import ModelRegistry
from utils import img_url

router = APIRouter()
registry = ModelRegistry()


@router.post("/identify")
async def identify_exhibit(
    museum_id: int = Form(...),
    image: UploadFile = File(...),
):
    """
    Принимает фото и museum_id, возвращает топ-3 совпадения экспонатов.
    """
    if not image.content_type.startswith("image/"):
        raise HTTPException(status_code=400, detail="Ожидается изображение")

    raw = await image.read()
    try:
        img = Image.open(io.BytesIO(raw)).convert("RGB")
    except Exception:
        raise HTTPException(status_code=400, detail="Не удалось декодировать изображение")

    model = registry.get(museum_id)
    if model is None:
        raise HTTPException(
            status_code=404,
            detail=f"Модель для музея {museum_id} не найдена. Обучите модель сначала.",
        )

    predictions = model.predict(img, top_k=3)

    if not predictions:
        return {"results": [], "museum_id": museum_id}

    # Обогащаем данными из БД
    exhibit_ids = [p["exhibit_id"] for p in predictions]
    placeholders = ",".join(f":id{i}" for i in range(len(exhibit_ids)))
    query = f"""
        SELECT e.id, e.title, COALESCE(e.description,'') AS description,
               e.image_url, ex.title AS exhibition_title
        FROM exhibits e
        JOIN exhibitions ex ON e.exhibition_id = ex.id
        WHERE e.id IN ({placeholders})
    """
    params = {f"id{i}": eid for i, eid in enumerate(exhibit_ids)}
    db_rows = await database.fetch_all(query, params)
    db_map = {r["id"]: dict(r) for r in db_rows}

    results = []
    for p in predictions:
        info = db_map.get(p["exhibit_id"], {})
        results.append({
            "exhibit_id": p["exhibit_id"],
            "confidence": round(p["confidence"], 4),
            "title": info.get("title", "Неизвестно"),
            "description": info.get("description", ""),
            "image_url": img_url(info.get("image_url")),
            "exhibition_title": info.get("exhibition_title", ""),
        })

    return {"results": results, "museum_id": museum_id}


@router.post("/train/{museum_id}")
async def trigger_training(museum_id: int):
    """
    Запускает (пере)обучение модели для музея в фоновом режиме.
    Используется из админки бота или веб-интерфейса.
    """
    from ml.train import train_museum_model
    import asyncio

    row = await database.fetch_one("SELECT id FROM museums WHERE id=:id", {"id": museum_id})
    if not row:
        raise HTTPException(status_code=404, detail="Музей не найден")

    count = await database.fetch_one(
        """SELECT COUNT(*) AS cnt FROM exhibits e
           JOIN exhibitions ex ON e.exhibition_id=ex.id
           WHERE ex.museum_id=:mid AND e.image_url IS NOT NULL AND e.is_active=true""",
        {"mid": museum_id},
    )
    if not count or count["cnt"] < 2:
        raise HTTPException(
            status_code=400,
            detail="Недостаточно экспонатов с фото (нужно минимум 2).",
        )

    asyncio.create_task(_run_training(museum_id))
    return {"status": "started", "museum_id": museum_id}


async def _run_training(museum_id: int):
    import asyncio
    from ml.train import train_museum_model

    loop = asyncio.get_event_loop()
    await loop.run_in_executor(None, train_museum_model, museum_id)

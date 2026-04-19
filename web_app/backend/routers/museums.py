from fastapi import APIRouter, HTTPException, Query
from typing import Optional
import math

from db import database
from utils import img_url

router = APIRouter()


@router.get("/")
async def list_museums():
    query = """
        SELECT id, name, COALESCE(NULLIF(short_name,''), name) AS display_name,
               COALESCE(description,'') AS description,
               COALESCE(address,'') AS address,
               latitude, longitude, image_url
        FROM museums ORDER BY name
    """
    rows = await database.fetch_all(query)
    result = [dict(r) for r in rows]
    for r in result:
        r["image_url"] = img_url(r.get("image_url"))
    return result


@router.get("/nearby")
async def nearby_museums(lat: float = Query(...), lon: float = Query(...)):
    query = """
        SELECT id, name, COALESCE(NULLIF(short_name,''), name) AS display_name,
               COALESCE(address,'') AS address,
               latitude, longitude, image_url
        FROM museums WHERE latitude IS NOT NULL AND longitude IS NOT NULL
    """
    rows = await database.fetch_all(query)
    results = []
    for r in rows:
        d = _haversine(lat, lon, r["latitude"], r["longitude"])
        results.append({**dict(r), "distance_km": round(d, 2)})
    results.sort(key=lambda x: x["distance_km"])
    for r in results:
        r["image_url"] = img_url(r.get("image_url"))
    return results[:5]


@router.get("/{museum_id}")
async def get_museum(museum_id: int):
    query = """
        SELECT id, name, COALESCE(NULLIF(short_name,''), name) AS display_name,
               COALESCE(description,'') AS description,
               COALESCE(address,'') AS address,
               COALESCE(opening_hours,'') AS opening_hours,
               COALESCE(phone,'') AS phone,
               COALESCE(website,'') AS website,
               latitude, longitude, image_url
        FROM museums WHERE id = :id
    """
    row = await database.fetch_one(query, {"id": museum_id})
    if not row:
        raise HTTPException(status_code=404, detail="Музей не найден")
    result = dict(row)
    result["image_url"] = img_url(result.get("image_url"))
    return result


@router.get("/{museum_id}/exhibits")
async def get_museum_exhibits(museum_id: int):
    """Возвращает все экспонаты музея (из всех его выставок), с флагом наличия модели."""
    query = """
        SELECT e.id, e.title, COALESCE(e.description,'') AS description,
               e.image_url, ex.title AS exhibition_title, ex.id AS exhibition_id
        FROM exhibits e
        JOIN exhibitions ex ON e.exhibition_id = ex.id
        WHERE ex.museum_id = :mid AND e.is_active = true
        ORDER BY ex.title, e.title
    """
    rows = await database.fetch_all(query, {"mid": museum_id})
    result = [dict(r) for r in rows]
    for r in result:
        r["image_url"] = img_url(r.get("image_url"))
    return result


@router.get("/{museum_id}/exhibitions")
async def get_museum_exhibitions(museum_id: int):
    query = """
        SELECT id, title, COALESCE(description,'') AS description,
               image_url, start_date::text, end_date::text
        FROM exhibitions
        WHERE museum_id = :mid AND is_active = true
        ORDER BY title
    """
    rows = await database.fetch_all(query, {"mid": museum_id})
    result = [dict(r) for r in rows]
    for r in result:
        r["image_url"] = img_url(r.get("image_url"))
    return result


@router.get("/{museum_id}/model-status")
async def model_status(museum_id: int):
    """Проверяет, обучена ли модель для этого музея."""
    import os
    model_path = os.path.join(
        os.path.dirname(__file__), f"../models/museum_{museum_id}.pt"
    )
    exists = os.path.exists(model_path)
    if exists:
        count_query = """
            SELECT COUNT(*) AS cnt FROM exhibits e
            JOIN exhibitions ex ON e.exhibition_id=ex.id
            WHERE ex.museum_id=:mid AND e.is_active=true
        """
        row = await database.fetch_one(count_query, {"mid": museum_id})
        num_classes = row["cnt"] if row else 0
    else:
        num_classes = 0
    return {"museum_id": museum_id, "model_ready": exists, "num_classes": num_classes}


def _haversine(lat1, lon1, lat2, lon2) -> float:
    R = 6371.0
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlam = math.radians(lon2 - lon1)
    a = math.sin(dphi / 2) ** 2 + math.cos(phi1) * math.cos(phi2) * math.sin(dlam / 2) ** 2
    return R * 2 * math.atan2(math.sqrt(a), math.sqrt(1 - a))

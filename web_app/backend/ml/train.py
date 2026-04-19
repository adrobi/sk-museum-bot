"""
Обучение (fine-tuning) EfficientNetB0 для экспонатов одного музея.

Стратегия:
  1. Скачиваем фото экспонатов из БД (image_url → PIL Image)
  2. Аугментация (flip, rotate, color jitter) для компенсации малого датасета
  3. Fine-tuning: сначала замораживаем backbone, обучаем голову, потом разморозка
  4. Сохраняем веса + метафайл (class → exhibit_id)

Запуск вручную:
    python -m ml.train --museum_id 1
"""
import os
import io
import json
import logging
import argparse
import asyncio
from pathlib import Path

import requests
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import Dataset, DataLoader
from torchvision import models, transforms
from PIL import Image

from db import database
from ml.model import MODELS_DIR, build_model

logger = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO)

MODELS_DIR_PATH = Path(MODELS_DIR)
MODELS_DIR_PATH.mkdir(parents=True, exist_ok=True)


TRAIN_TRANSFORMS = transforms.Compose([
    transforms.RandomResizedCrop(224, scale=(0.7, 1.0)),
    transforms.RandomHorizontalFlip(),
    transforms.RandomRotation(15),
    transforms.ColorJitter(brightness=0.3, contrast=0.3, saturation=0.2),
    transforms.ToTensor(),
    transforms.Normalize(mean=[0.485, 0.456, 0.406],
                         std=[0.229, 0.224, 0.225]),
])

VAL_TRANSFORMS = transforms.Compose([
    transforms.Resize(256),
    transforms.CenterCrop(224),
    transforms.ToTensor(),
    transforms.Normalize(mean=[0.485, 0.456, 0.406],
                         std=[0.229, 0.224, 0.225]),
])


class ExhibitDataset(Dataset):
    def __init__(self, samples: list[tuple], transform=None):
        # samples: [(pil_image, class_idx), ...]
        self.samples = samples
        self.transform = transform

    def __len__(self):
        return len(self.samples)

    def __getitem__(self, idx):
        img, label = self.samples[idx]
        if self.transform:
            img = self.transform(img)
        return img, label


def _load_local_images(exhibit_id: int, upload_dir: Path) -> list[Image.Image]:
    """Loads all uploaded training photos for an exhibit from local filesystem."""
    exhibit_dir = upload_dir / str(exhibit_id)
    if not exhibit_dir.exists():
        return []
    images = []
    for f in sorted(exhibit_dir.iterdir()):
        try:
            img = Image.open(f).convert("RGB")
            images.append(img)
        except Exception as e:
            logger.warning(f"Не удалось открыть {f}: {e}")
    return images


def _fetch_image(url: str) -> Image.Image | None:
    """Загружает изображение по URL или токену (Max API)."""
    if not url:
        return None
    if not url.startswith("http"):
        # Это токен Max Bot — строим URL для скачивания
        url = f"https://api.max.ru/files/{url}"
    try:
        resp = requests.get(url, timeout=10)
        resp.raise_for_status()
        return Image.open(io.BytesIO(resp.content)).convert("RGB")
    except Exception as e:
        logger.warning(f"Не удалось загрузить {url}: {e}")
        return None


async def _fetch_exhibit_data(museum_id: int) -> list[dict]:
    query = """
        SELECT e.id AS exhibit_id, e.title, e.image_url
        FROM exhibits e
        JOIN exhibitions ex ON e.exhibition_id = ex.id
        WHERE ex.museum_id = :mid AND e.is_active = true
              AND e.image_url IS NOT NULL AND e.image_url != ''
        ORDER BY e.id
    """
    rows = await database.fetch_all(query, {"mid": museum_id})
    return [dict(r) for r in rows]


async def _fetch_all_exhibits(museum_id: int) -> list[dict]:
    """Returns ALL active exhibits (with or without image_url) for upload-based training."""
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
    return [dict(r) for r in rows]


def _augment_images(images: list[Image.Image], target: int = 20) -> list[Image.Image]:
    """Наращивает датасет аугментацией до target штук на класс."""
    aug = transforms.Compose([
        transforms.RandomHorizontalFlip(),
        transforms.RandomRotation(20),
        transforms.ColorJitter(brightness=0.4, contrast=0.4),
        transforms.RandomResizedCrop(224, scale=(0.6, 1.0)),
    ])
    result = list(images)
    while len(result) < target:
        src = images[len(result) % len(images)]
        result.append(aug(src))
    return result


def train_museum_model(
    museum_id: int,
    epochs: int = 15,
    batch_size: int = 16,
    upload_dir: Path | None = None,
    progress_cb=None,
    exhibits: list[dict] | None = None,
):  # noqa: E501
    def _report(pct: int, msg: str):
        logger.info("[%d%%] %s", pct, msg)
        if progress_cb:
            progress_cb(pct, msg)

    _report(0, "Подготовка данных...")
    logger.info(f"=== Начало обучения для музея {museum_id} ===")

    if exhibits is None:
        # Standalone call (e.g. old /train endpoint): fetch from DB in a fresh event loop.
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        loop.run_until_complete(database.connect())
        if upload_dir is not None:
            exhibits = loop.run_until_complete(_fetch_all_exhibits(museum_id))
        else:
            exhibits = loop.run_until_complete(_fetch_exhibit_data(museum_id))
        loop.run_until_complete(database.disconnect())
        loop.close()

    if len(exhibits) < 2:
        logger.error("Недостаточно экспонатов (нужно минимум 2)")
        return False

    _report(5, f"Найдено экспонатов: {len(exhibits)}")

    # Load images: prefer local uploads, fall back to DB image_url
    exhibit_images: dict[int, list[Image.Image]] = {}
    total_ex = len(exhibits)
    for i, ex in enumerate(exhibits):
        eid = ex["exhibit_id"]
        imgs: list[Image.Image] = []
        if upload_dir:
            imgs = _load_local_images(eid, upload_dir)
        if not imgs and ex.get("image_url"):
            img = _fetch_image(ex["image_url"])
            if img:
                imgs = [img]
        if imgs:
            exhibit_images[eid] = imgs
        pct = 5 + int((i + 1) / total_ex * 15)
        _report(pct, f"Загрузка {i + 1}/{total_ex}: {ex['title']}")

    valid_exhibit_ids = [eid for eid, imgs in exhibit_images.items() if imgs]
    if len(valid_exhibit_ids) < 2:
        logger.error("Недостаточно загруженных изображений")
        return False

    _report(20, f"Экспонатов для обучения: {len(valid_exhibit_ids)}")

    class_to_exhibit = {i: eid for i, eid in enumerate(valid_exhibit_ids)}
    exhibit_to_class = {eid: i for i, eid in class_to_exhibit.items()}
    num_classes = len(valid_exhibit_ids)

    all_samples: list[tuple] = []
    for eid, imgs in exhibit_images.items():
        if eid not in exhibit_to_class:
            continue
        cls_idx = exhibit_to_class[eid]
        augmented = _augment_images(imgs, target=20)
        for img in augmented:
            all_samples.append((img, cls_idx))

    _report(25, f"Датасет: {len(all_samples)} образцов, {num_classes} классов")

    import random
    random.shuffle(all_samples)
    split = max(1, int(len(all_samples) * 0.8))
    train_samples = all_samples[:split]
    val_samples = all_samples[split:] if split < len(all_samples) else all_samples[:2]

    train_ds = ExhibitDataset(train_samples, transform=TRAIN_TRANSFORMS)
    val_ds = ExhibitDataset(val_samples, transform=VAL_TRANSFORMS)
    train_loader = DataLoader(train_ds, batch_size=batch_size, shuffle=True, num_workers=0)
    val_loader = DataLoader(val_ds, batch_size=batch_size, shuffle=False, num_workers=0)

    device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
    _report(28, f"Устройство: {device}")

    model = models.efficientnet_b0(weights=models.EfficientNet_B0_Weights.IMAGENET1K_V1)
    in_features = model.classifier[1].in_features
    model.classifier = nn.Sequential(
        nn.Dropout(p=0.3, inplace=True),
        nn.Linear(in_features, num_classes),
    )
    model = model.to(device)

    criterion = nn.CrossEntropyLoss()

    phase1_epochs = min(5, epochs)
    phase2_epochs = epochs - phase1_epochs

    # Phase 1: head-only (30→65%)
    for param in model.features.parameters():
        param.requires_grad = False
    optimizer = optim.Adam(model.classifier.parameters(), lr=1e-3)
    _run_epochs(model, train_loader, val_loader, criterion, optimizer, device,
                epochs=phase1_epochs, phase="Этап 1/2 (голова)",
                start_pct=30, end_pct=65, progress_cb=progress_cb)

    # Phase 2: full fine-tune (65→95%)
    for param in model.features.parameters():
        param.requires_grad = True
    optimizer = optim.AdamW(model.parameters(), lr=1e-4, weight_decay=1e-4)
    scheduler = optim.lr_scheduler.CosineAnnealingLR(optimizer, T_max=max(1, phase2_epochs))
    _run_epochs(model, train_loader, val_loader, criterion, optimizer, device,
                epochs=max(1, phase2_epochs), phase="Этап 2/2 (тонкая настройка)",
                scheduler=scheduler, start_pct=65, end_pct=95, progress_cb=progress_cb)

    _report(95, "Сохранение модели...")

    # Сохраняем
    model_path = MODELS_DIR_PATH / f"museum_{museum_id}.pt"
    meta_path = MODELS_DIR_PATH / f"museum_{museum_id}_meta.json"

    torch.save(model.state_dict(), model_path)
    with open(meta_path, "w") as f:
        json.dump({"class_to_exhibit": class_to_exhibit, "museum_id": museum_id,
                   "num_classes": num_classes}, f)

    logger.info(f"=== Модель сохранена: {model_path} ===")

    try:
        from routers.recognition import registry
        registry.invalidate(museum_id)
    except Exception:
        pass

    _report(100, "✅ Модель успешно обучена!")
    return True


def _run_epochs(model, train_loader, val_loader, criterion, optimizer, device,
                epochs: int, phase: str, scheduler=None,
                start_pct: int = 0, end_pct: int = 100, progress_cb=None):
    for epoch in range(epochs):
        model.train()
        total_loss, correct, total = 0.0, 0, 0
        for images, labels in train_loader:
            images, labels = images.to(device), labels.to(device)
            optimizer.zero_grad()
            outputs = model(images)
            loss = criterion(outputs, labels)
            loss.backward()
            optimizer.step()
            total_loss += loss.item()
            preds = outputs.argmax(dim=1)
            correct += (preds == labels).sum().item()
            total += labels.size(0)

        if scheduler:
            scheduler.step()

        val_acc = _validate(model, val_loader, device)
        train_acc = correct / total if total else 0.0
        pct = start_pct + int((epoch + 1) / epochs * (end_pct - start_pct))
        msg = (f"[{phase}] Эпоха {epoch+1}/{epochs} | "
               f"Loss: {total_loss/len(train_loader):.3f} | "
               f"Train: {train_acc:.1%} | Val: {val_acc:.1%}")
        logger.info(msg)
        if progress_cb:
            progress_cb(pct, msg)


def _validate(model, loader, device) -> float:
    model.eval()
    correct, total = 0, 0
    with torch.no_grad():
        for images, labels in loader:
            images, labels = images.to(device), labels.to(device)
            preds = model(images).argmax(dim=1)
            correct += (preds == labels).sum().item()
            total += labels.size(0)
    return correct / total if total > 0 else 0.0


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--museum_id", type=int, required=True)
    parser.add_argument("--epochs", type=int, default=15)
    args = parser.parse_args()
    train_museum_model(args.museum_id, epochs=args.epochs)

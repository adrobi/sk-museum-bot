"""
Inference: загрузка и кэширование обученных моделей по museum_id.
Каждая модель — EfficientNetB0 с дообученным классификатором.
"""
import os
import json
import logging
from typing import Optional

import torch
import torch.nn as nn
from torchvision import models, transforms
from PIL import Image

logger = logging.getLogger(__name__)

MODELS_DIR = os.path.join(os.path.dirname(__file__), "../models")

INFERENCE_TRANSFORMS = transforms.Compose([
    transforms.Resize(256),
    transforms.CenterCrop(224),
    transforms.ToTensor(),
    transforms.Normalize(mean=[0.485, 0.456, 0.406],
                         std=[0.229, 0.224, 0.225]),
])


def build_model(num_classes: int) -> nn.Module:
    """Создаёт EfficientNetB0 с кастомным классификатором."""
    model = models.efficientnet_b0(weights=None)
    in_features = model.classifier[1].in_features
    model.classifier = nn.Sequential(
        nn.Dropout(p=0.2, inplace=True),
        nn.Linear(in_features, num_classes),
    )
    return model


class MuseumModel:
    """Обёртка над обученной моделью одного музея."""

    def __init__(self, museum_id: int):
        self.museum_id = museum_id
        self.model: Optional[nn.Module] = None
        self.class_to_exhibit: dict[int, int] = {}  # class_idx → exhibit_id
        self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self._load()

    def _load(self):
        model_path = os.path.join(MODELS_DIR, f"museum_{self.museum_id}.pt")
        meta_path = os.path.join(MODELS_DIR, f"museum_{self.museum_id}_meta.json")
        if not os.path.exists(model_path) or not os.path.exists(meta_path):
            raise FileNotFoundError(f"Модель для музея {self.museum_id} не найдена")

        with open(meta_path) as f:
            meta = json.load(f)

        self.class_to_exhibit = {int(k): v for k, v in meta["class_to_exhibit"].items()}
        num_classes = len(self.class_to_exhibit)

        self.model = build_model(num_classes)
        state = torch.load(model_path, map_location=self.device, weights_only=True)
        self.model.load_state_dict(state)
        self.model.to(self.device)
        self.model.eval()
        logger.info(f"Модель музея {self.museum_id} загружена ({num_classes} классов)")

    def predict(self, image: Image.Image, top_k: int = 3) -> list[dict]:
        tensor = INFERENCE_TRANSFORMS(image).unsqueeze(0).to(self.device)
        with torch.no_grad():
            logits = self.model(tensor)
            probs = torch.softmax(logits, dim=1)[0]

        top_k = min(top_k, len(self.class_to_exhibit))
        topk_probs, topk_indices = torch.topk(probs, top_k)

        results = []
        for prob, idx in zip(topk_probs.tolist(), topk_indices.tolist()):
            exhibit_id = self.class_to_exhibit.get(idx)
            if exhibit_id is not None:
                results.append({"exhibit_id": exhibit_id, "confidence": prob})
        return results


class ModelRegistry:
    """Кэширует загруженные модели в памяти."""

    def __init__(self):
        self._cache: dict[int, MuseumModel] = {}

    def get(self, museum_id: int) -> Optional[MuseumModel]:
        if museum_id in self._cache:
            return self._cache[museum_id]
        try:
            m = MuseumModel(museum_id)
            self._cache[museum_id] = m
            return m
        except FileNotFoundError:
            return None
        except Exception as e:
            logger.error(f"Ошибка загрузки модели {museum_id}: {e}")
            return None

    def invalidate(self, museum_id: int):
        """Сбрасывает кэш после переобучения."""
        self._cache.pop(museum_id, None)
        logger.info(f"Кэш модели музея {museum_id} сброшен")

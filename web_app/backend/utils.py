from urllib.parse import quote


def img_url(raw: str | None) -> str | None:
    """Converts a Max messenger photo token to a public CDN URL.
    If raw already starts with http, returns as-is.
    Tokens are standard base64 (may contain +, /) and must be URL-encoded.
    """
    if not raw:
        return None
    if raw.startswith("http"):
        return raw
    return f"https://i.oneme.ru/i?r={quote(raw, safe='')}"

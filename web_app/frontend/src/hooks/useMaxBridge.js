import { useEffect, useMemo, useState } from "react";

const EMPTY_BRIDGE_STATE = {
  raw: null,
  isMiniApp: false,
  initData: "",
  initDataUnsafe: {},
  user: null,
  startParam: null,
  platform: "web",
  version: "",
};

function parseInitData(initData) {
  if (!initData) return {};

  const result = {};
  for (const chunk of initData.split("&")) {
    if (!chunk) continue;

    const separatorIndex = chunk.indexOf("=");
    const key = separatorIndex >= 0 ? chunk.slice(0, separatorIndex) : chunk;
    const rawValue = separatorIndex >= 0 ? chunk.slice(separatorIndex + 1) : "";
    if (!key) continue;

    let value = rawValue;
    try {
      value = decodeURIComponent(rawValue);
    } catch {
    }

    if (key === "user" || key === "chat") {
      try {
        result[key] = JSON.parse(value);
      } catch {
        result[key] = null;
      }
      continue;
    }

    result[key] = value;
  }

  return result;
}

function readBridgeState() {
  if (typeof window === "undefined") return EMPTY_BRIDGE_STATE;

  const wa = window.WebApp ?? null;
  const hash = window.location.hash.startsWith("#") ? window.location.hash.slice(1) : window.location.hash;
  const hashParams = new URLSearchParams(hash);
  const hashInitData = hashParams.get("WebAppData") || "";
  const hashPlatform = hashParams.get("WebAppPlatform") || "";
  const hashVersion = hashParams.get("WebAppVersion") || "";

  const bridgeInitData = typeof wa?.initData === "string" ? wa.initData : "";
  const bridgePlatform = typeof wa?.platform === "string" ? wa.platform : "";
  const hasBridgeUser = typeof wa?.initDataUnsafe?.user?.id === "number";
  const hasBridgeContext = Boolean(
    bridgeInitData || hasBridgeUser || (bridgePlatform && bridgePlatform !== "web")
  );
  const hasHashContext = Boolean(hashInitData || hashPlatform);
  const isMiniApp = hasBridgeContext || (!wa && hasHashContext);

  const initData = bridgeInitData || (!wa ? hashInitData : "");
  const initDataUnsafe = wa?.initDataUnsafe && Object.keys(wa.initDataUnsafe).length > 0
    ? wa.initDataUnsafe
    : parseInitData(initData);

  return {
    raw: wa,
    isMiniApp,
    initData,
    initDataUnsafe,
    user: initDataUnsafe.user || null,
    startParam: initDataUnsafe.start_param || null,
    platform: bridgePlatform || (!wa && isMiniApp ? hashPlatform : "web"),
    version: wa?.version || (!wa && isMiniApp ? hashVersion : ""),
  };
}

function isSameBridgeState(a, b) {
  return (
    a.raw === b.raw &&
    a.isMiniApp === b.isMiniApp &&
    a.initData === b.initData &&
    a.startParam === b.startParam &&
    a.platform === b.platform &&
    a.version === b.version &&
    (a.user?.id ?? null) === (b.user?.id ?? null) &&
    (a.user?.first_name ?? "") === (b.user?.first_name ?? "") &&
    (a.user?.username ?? "") === (b.user?.username ?? "")
  );
}

/**
 * Хелпер для работы с MAX Bridge.
 * Если приложение открыто как мини-апп в MAX — window.WebApp доступен.
 * Если как обычный сайт в браузере — window.WebApp undefined, все методы-заглушки.
 */
export default function useMaxBridge() {
  const [state, setState] = useState(() => readBridgeState());

  useEffect(() => {
    if (typeof window === "undefined") return undefined;

    let attempts = 0;
    const refresh = () => {
      const next = readBridgeState();
      setState((prev) => (isSameBridgeState(prev, next) ? prev : next));
      return next;
    };

    const handleVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        refresh();
      }
    };

    refresh();
    const intervalId = window.setInterval(() => {
      attempts += 1;
      const next = refresh();
      if ((next.isMiniApp && next.initData && next.user) || attempts >= 20) {
        window.clearInterval(intervalId);
      }
    }, 250);

    window.addEventListener("hashchange", refresh);
    window.addEventListener("focus", refresh);
    window.addEventListener("pageshow", refresh);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      window.clearInterval(intervalId);
      window.removeEventListener("hashchange", refresh);
      window.removeEventListener("focus", refresh);
      window.removeEventListener("pageshow", refresh);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, []);

  return useMemo(() => {
    const wa = state.raw;
    const isMiniApp = state.isMiniApp;

    // BackButton
    const showBackButton = () => {
      if (isMiniApp && wa?.BackButton) wa.BackButton.show();
    };
    const hideBackButton = () => {
      if (isMiniApp && wa?.BackButton) wa.BackButton.hide();
    };
    const onBackButton = (cb) => {
      if (isMiniApp && wa?.BackButton) wa.BackButton.onClick(cb);
    };
    const offBackButton = (cb) => {
      if (isMiniApp && wa?.BackButton) wa.BackButton.offClick(cb);
    };

    // Открытие внешней ссылки
    const openLink = (url) => {
      if (isMiniApp && wa?.openLink) {
        wa.openLink(url);
      } else {
        window.open(url, "_blank", "noopener,noreferrer");
      }
    };

    // Закрытие мини-аппа
    const close = () => {
      if (isMiniApp && wa?.close) wa.close();
    };

    // Haptic feedback
    const haptic = (type = "light") => {
      if (isMiniApp && wa?.HapticFeedback) {
        wa.HapticFeedback.impactOccurred(type);
      }
    };

    return {
      isMiniApp,
      user: state.user,
      startParam: state.startParam,
      platform: state.platform,
      version: state.version,
      initData: state.initData,
      showBackButton,
      hideBackButton,
      onBackButton,
      offBackButton,
      openLink,
      close,
      haptic,
      raw: wa,
    };
  }, [state]);
}

import { useMemo } from "react";

/**
 * Хелпер для работы с MAX Bridge.
 * Если приложение открыто как мини-апп в MAX — window.WebApp доступен.
 * Если как обычный сайт в браузере — window.WebApp undefined, все методы-заглушки.
 */
export default function useMaxBridge() {
  return useMemo(() => {
    const wa = typeof window !== "undefined" ? window.WebApp : undefined;

    // Приложение запущено внутри MAX (мини-апп), если WebApp существует
    // и есть initData (непустая строка) или platform отличен от undefined.
    const isMiniApp = !!(wa && (wa.initData || wa.platform));

    // initDataUnsafe содержит user, chat, start_param и т.д.
    const initData = isMiniApp ? wa.initDataUnsafe || {} : {};
    const user = initData.user || null;
    const startParam = initData.start_param || null;
    const platform = isMiniApp ? wa.platform : "web";

    // BackButton
    const showBackButton = () => {
      if (isMiniApp && wa.BackButton) wa.BackButton.show();
    };
    const hideBackButton = () => {
      if (isMiniApp && wa.BackButton) wa.BackButton.hide();
    };
    const onBackButton = (cb) => {
      if (isMiniApp && wa.BackButton) wa.BackButton.onClick(cb);
    };
    const offBackButton = (cb) => {
      if (isMiniApp && wa.BackButton) wa.BackButton.offClick(cb);
    };

    // Открытие внешней ссылки
    const openLink = (url) => {
      if (isMiniApp && wa.openLink) {
        wa.openLink(url);
      } else {
        window.open(url, "_blank", "noopener,noreferrer");
      }
    };

    // Закрытие мини-аппа
    const close = () => {
      if (isMiniApp && wa.close) wa.close();
    };

    // Haptic feedback
    const haptic = (type = "light") => {
      if (isMiniApp && wa.HapticFeedback) {
        wa.HapticFeedback.impactOccurred(type);
      }
    };

    return {
      isMiniApp,
      user,
      startParam,
      platform,
      showBackButton,
      hideBackButton,
      onBackButton,
      offBackButton,
      openLink,
      close,
      haptic,
      raw: wa,
    };
  }, []);
}

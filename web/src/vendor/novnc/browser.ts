const hasWindow = typeof window !== "undefined";
const hasDocument = typeof document !== "undefined";
const navigatorRef = typeof navigator !== "undefined" ? navigator : null;

const platform = navigatorRef?.platform ?? "";
const userAgent = navigatorRef?.userAgent ?? "";

const hasTouch =
  hasWindow &&
  hasDocument &&
  ("ontouchstart" in document.documentElement ||
    // required for Chrome debugger
    (document as Document & { ontouchstart?: unknown }).ontouchstart !==
      undefined ||
    // required for MS Surface
    (navigatorRef?.maxTouchPoints ?? 0) > 0 ||
    ((navigatorRef as Navigator & { msMaxTouchPoints?: number } | null)
      ?.msMaxTouchPoints ?? 0) > 0);

export let isTouchDevice = hasTouch;

if (hasWindow && hasDocument) {
  window.addEventListener(
    "touchstart",
    function onFirstTouch() {
      isTouchDevice = true;
      window.removeEventListener("touchstart", onFirstTouch, false);
    },
    false,
  );
}

export const dragThreshold = 10 * (hasWindow ? window.devicePixelRatio || 1 : 1);

let cursorURISupported = false;
if (hasDocument) {
  try {
    const target = document.createElement("canvas");
    target.style.cursor =
      'url("data:image/x-icon;base64,AAACAAEACAgAAAIAAgA4AQAAFgAAACgAAAAIAAAAEAAAAAEAIAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAD/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////AAAAAAAAAAAAAAAAAAAAAA==") 2 2, default';
    cursorURISupported = target.style.cursor.indexOf("url") === 0;
  } catch {
    cursorURISupported = false;
  }
}

export const supportsCursorURIs = cursorURISupported;

let scrollbarGutter = true;
if (hasDocument && document.body) {
  try {
    const container = document.createElement("div");
    container.style.visibility = "hidden";
    container.style.overflow = "scroll";
    document.body.appendChild(container);

    const child = document.createElement("div");
    container.appendChild(child);
    scrollbarGutter = container.offsetWidth - child.offsetWidth !== 0;
    container.parentNode?.removeChild(container);
  } catch {
    scrollbarGutter = true;
  }
}

export const hasScrollbarGutter = scrollbarGutter;

// Keep the upstream export surface, but avoid top-level await during bundling.
// noVNC treats this as an optional acceleration capability.
export const supportsWebCodecsH264Decode = false;

export function isMac(): boolean {
  return /mac/i.test(platform);
}

export function isWindows(): boolean {
  return /win/i.test(platform);
}

export function isIOS(): boolean {
  return /ipad/i.test(platform) || /iphone/i.test(platform) || /ipod/i.test(platform);
}

export function isAndroid(): boolean {
  return userAgent.includes("Android ");
}

export function isChromeOS(): boolean {
  return userAgent.includes(" CrOS ");
}

export function isSafari(): boolean {
  return /Safari\/.../.test(userAgent) && !/Chrome\/.../.test(userAgent) && !/Chromium\/.../.test(userAgent) && !/Epiphany\/.../.test(userAgent);
}

export function isFirefox(): boolean {
  return /Firefox\/.../.test(userAgent) && !/Seamonkey\/.../.test(userAgent);
}

export function isChrome(): boolean {
  return /Chrome\/.../.test(userAgent) && !/Chromium\/.../.test(userAgent) && !/Edg\/.../.test(userAgent) && !/OPR\/.../.test(userAgent);
}

export function isChromium(): boolean {
  return /Chromium\/.../.test(userAgent);
}

export function isOpera(): boolean {
  return /OPR\/.../.test(userAgent);
}

export function isEdge(): boolean {
  return /Edg\/.../.test(userAgent);
}

export function isGecko(): boolean {
  return /Gecko\/.../.test(userAgent);
}

export function isWebKit(): boolean {
  return /AppleWebKit\/.../.test(userAgent) && !/Chrome\/.../.test(userAgent);
}

export function isBlink(): boolean {
  return /Chrome\/.../.test(userAgent);
}

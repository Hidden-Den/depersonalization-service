import { SetClipboardText } from '../api/wails';

/**
 * Universal cross-platform clipboard copy utility.
 * Guarantees reliable text copying across all desktop webviews (WebKitGTK on Linux,
 * WebView2 on Windows, WKWebView on macOS) as well as modern browsers,
 * using a 3-layer fallback mechanism:
 * 1. Go backend (runtime.ClipboardSetText via native OS APIs).
 * 2. Modern browser navigator.clipboard.writeText.
 * 3. Legacy document.execCommand('copy') via temporary textarea.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false;

  // Layer 1: Wails backend native desktop clipboard
  try {
    const success = await SetClipboardText(text);
    if (success) {
      return true;
    }
  } catch (err) {
    console.warn('[Clipboard] Backend SetClipboardText error:', err);
  }

  // Layer 2: Standard Browser Clipboard API
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (err) {
    console.warn('[Clipboard] navigator.clipboard.writeText restricted or failed:', err);
  }

  // Layer 3: Fallback DOM execCommand('copy')
  try {
    if (typeof document !== 'undefined') {
      const textArea = document.createElement('textarea');
      textArea.value = text;
      textArea.style.position = 'fixed';
      textArea.style.left = '-9999px';
      textArea.style.top = '-9999px';
      textArea.style.opacity = '0';
      textArea.setAttribute('readonly', '');
      document.body.appendChild(textArea);
      textArea.focus();
      textArea.select();
      textArea.setSelectionRange(0, textArea.value.length);
      const successful = document.execCommand('copy');
      document.body.removeChild(textArea);
      if (successful) {
        return true;
      }
    }
  } catch (err) {
    console.error('[Clipboard] document.execCommand fallback failed:', err);
  }

  return false;
}

import { useCallback, useContext, useRef, useState } from "react";
import { ToastContext } from "../contexts/ToastContext";

interface UseClipboardOptions {
  successMessage?: string;
  errorMessage?: string;
  timeout?: number;
}

function fallbackCopyText(text: string): boolean {
  let textArea: HTMLTextAreaElement | null = null;
  try {
    textArea = document.createElement("textarea");
    textArea.value = text;

    // Prevent scrolling and keep offscreen
    textArea.style.position = "fixed";
    textArea.style.left = "-9999px";
    textArea.style.top = "-9999px";
    textArea.style.width = "2em";
    textArea.style.height = "2em";
    textArea.style.padding = "0";
    textArea.style.border = "none";
    textArea.style.outline = "none";
    textArea.style.boxShadow = "none";
    textArea.style.background = "transparent";
    textArea.style.opacity = "0";
    textArea.setAttribute("readonly", "");

    document.body.appendChild(textArea);

    // Handle iOS Safari & mobile selection quirks
    if (typeof navigator !== "undefined" && navigator.userAgent.match(/ipad|ipod|iphone/i)) {
      const editable = textArea.contentEditable;
      const readOnly = textArea.readOnly;

      textArea.contentEditable = "true";
      textArea.readOnly = false;

      const range = document.createRange();
      range.selectNodeContents(textArea);

      const selection = window.getSelection();
      if (selection) {
        selection.removeAllRanges();
        selection.addRange(range);
      }
      textArea.setSelectionRange(0, 999999);
      textArea.contentEditable = editable;
      textArea.readOnly = readOnly;
    } else {
      textArea.focus();
      textArea.select();
      textArea.setSelectionRange(0, text.length);
    }

    const successful = document.execCommand("copy");
    return successful;
  } catch (err) {
    console.warn("Fallback execCommand copy failed:", err);
    return false;
  } finally {
    if (textArea && textArea.parentNode) {
      textArea.parentNode.removeChild(textArea);
    }
  }
}

/**
 * Universal clipboard copy function with fallback for non-secure remote contexts (HTTP over LAN).
 */
export async function copyTextToClipboard(text: string): Promise<boolean> {
  // If in a secure context (HTTPS / localhost / Wails), use modern Async Clipboard API
  const isSecure = typeof window !== "undefined" && window.isSecureContext && Boolean(navigator?.clipboard?.writeText);

  if (isSecure) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (e) {
      console.warn("navigator.clipboard failed, attempting fallback:", e);
      return fallbackCopyText(text);
    }
  }

  // In non-secure context (HTTP LAN remote access), execute synchronous execCommand immediately
  // while user gesture / activation is active.
  return fallbackCopyText(text);
}

export function useClipboard(options: UseClipboardOptions = {}) {
  const {
    successMessage = "Copied to clipboard",
    errorMessage = "Failed to copy",
    timeout = 2000,
  } = options;

  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<number | null>(null);
  const toastContext = useContext(ToastContext);

  const copy = useCallback(
    async (text: string) => {
      const ok = await copyTextToClipboard(text);
      if (ok) {
        setCopied(true);

        if (timeoutRef.current !== null) {
          clearTimeout(timeoutRef.current);
        }

        timeoutRef.current = window.setTimeout(() => {
          setCopied(false);
        }, timeout);

        if (successMessage) {
          toastContext.actions.showToast(successMessage, "success");
        }
        return true;
      } else {
        if (errorMessage) {
          toastContext.actions.showToast(errorMessage, "danger");
        }
        return false;
      }
    },
    [successMessage, errorMessage, timeout, toastContext],
  );

  return { copied, copy };
}

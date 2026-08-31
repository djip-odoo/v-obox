import { useCallback, useContext, useRef, useState } from "react";
import { ToastContext } from "../contexts/ToastContext";

interface UseClipboardOptions {
  successMessage?: string;
  errorMessage?: string;
  timeout?: number;
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
      try {
        await navigator.clipboard.writeText(text);
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
      } catch {
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

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useMountTransition } from "../hooks/useMountTransition";
import CloseButton from "./CloseButton";

interface DialogProps {
  title: string;
  validateText?: string;
  openButton?: React.ReactNode;
  children: React.ReactNode;
  isValidateDisabled?: boolean;
  validateCallback?: () => Promise<boolean>;
  onClose?: () => void;
  onOpen?: () => void;
}

export default function Dialog({
  title,
  children,
  openButton,
  validateText,
  isValidateDisabled,
  validateCallback,
  onClose,
  onOpen,
}: DialogProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const { mounted } = useMountTransition(isOpen);

  const close = () => {
    setIsOpen(false);
    onClose?.();
  };

  const open = () => {
    setIsOpen(true);
    onOpen?.();
  };

  const isValidating = useRef(false);

  const validate = async () => {
    if (!validateCallback) {
      close();
      return;
    }

    if (isValidating.current) {
      return;
    }

    isValidating.current = true;
    setIsLoading(true);
    try {
      if (await validateCallback()) {
        close();
      }
    } finally {
      isValidating.current = false;
      setIsLoading(false);
    }
  };

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        close();
        return;
      }

      if (event.key !== "Enter" || event.repeat) {
        return;
      }

      if ((event.target as HTMLElement | null)?.closest("button, a")) {
        return;
      }

      if (validateText && !isValidateDisabled && !isLoading) {
        validate();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [
    isOpen,
    isLoading,
    isValidateDisabled,
    validateText,
    validateCallback,
    onClose,
  ]);

  return (
    <>
      {openButton && (
          <div onClick={() => open()}>{openButton}</div>
      )}

      {mounted &&
        createPortal(
          <div
            className={`fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4 transition ${
              isOpen
                ? "opacity-100 duration-200 ease-out"
                : "opacity-0 duration-150 ease-in"
            }`}
          >
            <div
              className="absolute inset-0 bg-black/75"
              onClick={() => close()}
            />

            <div className="relative bg-white rounded-2xl w-full max-w-sm shadow-xl overflow-hidden p-6">
              <div className="flex items-center justify-between mb-4">
                <div className="text-lg font-medium">{title}</div>
                <CloseButton onClick={() => close()} />
              </div>
              <div>{children}</div>
              <div className="">
                {validateText && (
                  <button
                    disabled={isLoading || isValidateDisabled === true}
                    className="w-full border rounded-lg px-4 py-2 cursor-pointer text-sm bg-odoo text-white hover:bg-odoo-dark disabled:opacity-50 disabled:cursor-not-allowed"
                    onClick={() => validate()}
                  >
                    {isLoading ? "Loading..." : validateText}
                  </button>
                )}
              </div>
            </div>
          </div>,
          document.body,
        )}
    </>
  );
}

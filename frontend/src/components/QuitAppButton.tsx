import { useContext, useState } from "react";
import { createPortal } from "react-dom";
import { WebViewContext } from "../contexts/WebViewContext";
import { AppContext } from "../contexts/AppContext";
import { backendService } from "../services/backend";
import PINModal from "./PINModal";

export default function QuitAppButton() {
  const { data: { config } } = useContext(WebViewContext);
  const { data: { isWails } } = useContext(AppContext);
  const [showPinModal, setShowPinModal] = useState(false);
  const [showConfirmDialog, setShowConfirmDialog] = useState(false);
  const [isQuitting, setIsQuitting] = useState(false);

  const handleQuitClick = () => {
    if (config?.hasPIN) {
      setShowPinModal(true);
    } else {
      setShowConfirmDialog(true);
    }
  };

  const executeQuit = async () => {
    try {
      setIsQuitting(true);
      await backendService.quitApp();
    } catch (err) {
      console.error("Failed to quit application:", err);
      setIsQuitting(false);
    }
  };

  return (
    <>{isWails &&
      <button
        type="button"
        onClick={handleQuitClick}
        disabled={isQuitting}
        className="w-full flex items-center justify-center gap-2 py-3 px-4 rounded-xl border border-red-200 bg-red-50/80 hover:bg-red-100/90 text-red-700 font-medium text-sm transition-all duration-200 shadow-sm active:scale-[0.99]"
      >
        <svg
          className="w-4 h-4 text-red-600"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth="2"
            d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
          />
        </svg>
        <span>{isQuitting ? "Closing App..." : "Quit Application"}</span>
      </button>}

      {showPinModal && (
        <PINModal
          mode="auth"
          title="Quit Application"
          subtitle="Enter PIN to close the application"
          onSuccess={() => {
            setShowPinModal(false);
            executeQuit();
          }}
          onDismiss={() => setShowPinModal(false)}
        />
      )}

      {showConfirmDialog &&
        createPortal(
          <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-xs">
            <div className="bg-white rounded-2xl shadow-xl max-w-sm w-full p-6 text-left border border-gray-100 animate-in fade-in zoom-in-95 duration-150">
              <h3 className="text-lg font-semibold text-gray-900 mb-2">
                Quit Application
              </h3>
              <p className="text-sm text-gray-600 mb-6">
                Stopping the proxy will prevent POS from printing receipts. Are you sure you want to quit?
              </p>
              <div className="flex gap-3 justify-end">
                <button
                  type="button"
                  onClick={() => setShowConfirmDialog(false)}
                  className="px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 hover:bg-gray-200 rounded-xl transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setShowConfirmDialog(false);
                    executeQuit();
                  }}
                  className="px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-xl transition-colors"
                >
                  Quit App
                </button>
              </div>
            </div>
          </div>,
          document.body
        )}
    </>
  );
}

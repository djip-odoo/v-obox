import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
} from "react";
import PINModal from "../components/PINModal";

/**
 * PINContext — provides a programmatic `showPINDialog()` that returns a
 * Promise<boolean>. Components use this (via usePINGate) to gate privileged
 * actions behind the 4-digit PIN in remote context.
 *
 * A single PINModal instance is mounted here so it is always above all other
 * content in the z-order regardless of where the action is triggered from.
 */

type PINContextType = {
  /** Opens the PIN dialog and resolves to true on success, false on dismiss. */
  showPINDialog: () => Promise<boolean>;
};

export const PINContext = createContext<PINContextType>({
  showPINDialog: async () => false,
});

export function usePINContext() {
  return useContext(PINContext);
}

interface PINContextWrapperProps {
  children: React.ReactNode;
}

export function PINContextWrapper({ children }: PINContextWrapperProps) {
  const [visible, setVisible] = useState(false);
  // Store the resolve function of the current in-flight promise.
  const resolverRef = useRef<((ok: boolean) => void) | null>(null);

  const showPINDialog = useCallback((): Promise<boolean> => {
    return new Promise<boolean>((resolve) => {
      resolverRef.current = resolve;
      setVisible(true);
    });
  }, []);

  const handleSuccess = useCallback(() => {
    setVisible(false);
    resolverRef.current?.(true);
    resolverRef.current = null;
  }, []);

  const handleDismiss = useCallback(() => {
    setVisible(false);
    resolverRef.current?.(false);
    resolverRef.current = null;
  }, []);

  return (
    <PINContext.Provider value={{ showPINDialog }}>
      {children}
      {visible && (
        <PINModal onSuccess={handleSuccess} onDismiss={handleDismiss} />
      )}
    </PINContext.Provider>
  );
}

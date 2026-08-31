import { useContext, useEffect, useState } from "react";
import { EventsOn } from "../../wailsjs/runtime/runtime";
import { RuntimeContext } from "../contexts/RuntimeContext";
import PINModal from "./PINModal";

export default function SetPinDialog() {
  const { isWails } = useContext(RuntimeContext);
  const [isOpen, setIsOpen] = useState(false);

  useEffect(() => {
    if (!isWails) return;

    return EventsOn("open-set-pin-dialog", () => {
      setIsOpen(true);
    });
  }, [isWails]);

  if (!isWails || !isOpen) {
    return null;
  }

  return (
    <PINModal
      mode="set"
      onSuccess={() => setIsOpen(false)}
      onDismiss={() => setIsOpen(false)}
    />
  );
}

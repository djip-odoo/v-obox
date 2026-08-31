import { useContext, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { AppContext } from "../contexts/AppContext";
import PINModal from "./PINModal";

export default function SetPinDialog() {
  const { data: { isWails } } = useContext(AppContext);
  const [isOpen, setIsOpen] = useState(false);

  useEffect(() => {
    if (!isWails) return;

    return Events.On("open-set-pin-dialog", () => {
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

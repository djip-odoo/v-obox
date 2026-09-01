import { useContext, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { backendService } from "../services/backend";
import { staticIpAdvice } from "../assets/data/troubleshootStep";
import { renderFormattedText } from "../functions/renderFormattedText";
import Dialog, { type ActionType } from "./Dialog";
import { AppContext } from "../contexts/AppContext";

export default function NetworkPrintingEnabledDialog() {
  const { data: { isWails } } = useContext(AppContext);
  const [openSignal, setOpenSignal] = useState(0);
  const [localIp, setLocalIp] = useState<string | null>(null);

  useEffect(() => {
    if (!isWails) return;

    return Events.On("network-printing-changed", (ev: { data: boolean }) => {
      const enabled = ev.data;
      if (!enabled) {
        return;
      }

      setOpenSignal((count) => count + 1);
      backendService
        .getTroubleshootInfo()
        .then((info) => setLocalIp(info?.localIp ?? null))
        .catch((err) => console.error("Failed to load troubleshoot info", err));
    });
  }, [isWails]);

  if (!isWails) {
    return null;
  }

  return (
    <Dialog
      title="Allow Other Devices to Print"
      openSignal={openSignal}
      actions={[{ name: "ok", label: "Got it", variant: "primary" as ActionType }]}
    >
      <p className="text-gray-600 text-sm leading-relaxed">
        Devices on this network will be able to send print jobs to your printers.
      </p>
      {localIp && (
        <p className="text-gray-600 text-sm whitespace-pre-line leading-relaxed mt-3">
          {renderFormattedText(staticIpAdvice(localIp))}
        </p>
      )}
    </Dialog>
  );
}

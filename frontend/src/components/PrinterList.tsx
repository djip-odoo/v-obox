import { useContext } from "react";
import NetworkIpDialog from "./NetworkIpDialog";
import PrinterListItem from "./PrinterListItem";
import { PrinterContext } from "../contexts/PrinterContext";
import WebViewDialog from "./WebViewDialog";
import QuitAppButton from "./QuitAppButton";
import DeviceInfoDialog from "./DeviceInfoDialog";

export default function PrinterList() {
  const printerContext = useContext(PrinterContext);
  const { printers, fetchError } = printerContext.data;
  const errorMessage = fetchError ?? printers?.errorMsg;

  return (
    <>
      <div className="w-full max-w-full sm:max-w-md md:max-w-lg lg:max-w-xl bg-white/95 rounded-2xl shadow-sm border border-gray-200/80 overflow-hidden p-4 sm:p-6">
        {printers && (printers.printers.length > 0 || printers.unavailablePrinters.length > 0) && (
          <div>
            <ul className="divide-y divide-gray-200">
              {printers.printers.map((printer) => (
                <PrinterListItem
                  key={printer.id}
                  printer={printer}
                  isOnline={true}
                />
              ))}
              {printers.unavailablePrinters.map((printer) => (
                <PrinterListItem
                  key={printer.name}
                  printer={printer}
                  isOnline={false}
                />
              ))}
            </ul>
          </div>
        )}

        {!printers ? (
          !fetchError && (
            <div className="py-6 text-center">
              <div className="font-medium text-base sm:text-lg text-gray-700">
                Searching for printers...
              </div>
            </div>
          )
        ) : (
          printers.printers.length === 0 &&
          printers.unavailablePrinters.length === 0 && (
            <div className="py-6 text-center">
              <div className="font-medium text-base sm:text-lg text-gray-700">
                No printers found
              </div>
              <div className="mt-2 text-xs sm:text-sm text-gray-500">
                Make sure your printer is powered on and connected via USB or LAN.
              </div>
            </div>
          )
        )}

        {errorMessage && (
          <div className="text-danger text-xs sm:text-sm mt-3 text-center bg-red-50 border border-red-200 rounded-lg p-2.5">
            Error: {errorMessage}
          </div>
        )}
      </div>

      <div className="mt-4 sm:mt-6 w-full flex flex-col gap-3 max-w-full sm:max-w-md md:max-w-lg lg:max-w-xl">
        <NetworkIpDialog />
        <WebViewDialog />
        <QuitAppButton />
        <DeviceInfoDialog />
      </div>
    </>
  );
}

import KioskOverlay from "./components/KioskOverlay";
import NetworkPrinting from "./components/NetworkPrinting";
import NetworkPrintingEnabledDialog from "./components/NetworkPrintingEnabledDialog";
import PrinterList from "./components/PrinterList";
import SetPinDialog from "./components/SetPinDialog";
import { AppContextWrapper } from "./contexts/AppContext";
import { PINContextWrapper } from "./contexts/PINContext";
import { PrinterContextWrapper } from "./contexts/PrinterContext";
import { RuntimeContextWrapper } from "./contexts/RuntimeContext";
import { ToastContextWrapper } from "./contexts/ToastContext";
import { WebViewContextWrapper } from "./contexts/WebViewContext";

function App() {
  return (
    <RuntimeContextWrapper>
      <ToastContextWrapper>
        <AppContextWrapper>
          <WebViewContextWrapper>
            <PINContextWrapper>
              <PrinterContextWrapper>
                <div className="min-h-screen w-full flex flex-col items-center justify-start sm:justify-center py-5 px-3.5 sm:p-6 font-sans bg-gray-50 overflow-x-hidden">
                  <PrinterList />
                  <NetworkPrintingEnabledDialog />
                  <NetworkPrinting />
                  <SetPinDialog />
                </div>
                {/* KioskOverlay is full-screen; render outside the padded container */}
                <KioskOverlay />
              </PrinterContextWrapper>
            </PINContextWrapper>
          </WebViewContextWrapper>
        </AppContextWrapper>
      </ToastContextWrapper>
    </RuntimeContextWrapper>
  );
}

export default App;

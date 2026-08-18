import { useEffect, useState } from "react";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import type { Step } from "../types";
import { useStepDialog } from "../hooks/useStepDialog";
import Dialog from "./Dialog";

type ContentPhase = "shown" | "leaving" | "entering";

const CONTENT_PHASE_CLASS: Record<ContentPhase, string> = {
  leaving: "opacity-0 -translate-x-4 transition duration-200 ease-in",
  entering: "opacity-0 translate-x-4",
  shown: "opacity-100 translate-x-0 transition duration-300 ease-out",
};

interface StepDialogProps {
  steps: Step[];
  openButton: React.ReactNode;
  title?: string;
  onOpen?: () => void;
}

function renderFormattedText(text: string) {
  if (!text) return null;
  const parts = text.split(/\*([^*]+)\*/g);
  return parts.map((part, index) =>
    index % 2 === 1 ? (
      <span key={index} className="font-bold text-odoo">
        {part}
      </span>
    ) : (
      part
    ),
  );
}

export default function StepDialog({
  steps,
  openButton,
  title,
  onOpen,
}: StepDialogProps) {
  const [contentEl, setContentEl] = useState<HTMLDivElement | null>(null);
  const { currentStep, next, back, setCurrentStep } = useStepDialog();
  const [displayStep, setDisplayStep] = useState(0);
  const [phase, setPhase] = useState<ContentPhase>("shown");
  const [contentHeight, setContentHeight] = useState("auto");
  const [codeCopied, setCodeCopied] = useState<Record<number, boolean>>({});

  useEffect(() => {
    if (currentStep === displayStep) {
      return;
    }

    setPhase("leaving");
    const timeout = setTimeout(() => {
      setDisplayStep(currentStep);
      setPhase("entering");
    }, 200);

    return () => clearTimeout(timeout);
  }, [currentStep, displayStep]);

  useEffect(() => {
    if (!contentEl) {
      // Drop the pinned height so the next open measures from scratch instead
      // of clipping the first step against the last one's height.
      setContentHeight("auto");
      return;
    }

    const observer = new ResizeObserver(() =>
      setContentHeight(`${contentEl.offsetHeight}px`),
    );

    observer.observe(contentEl);
    return () => observer.disconnect();
  }, [contentEl]);

  useEffect(() => {
    if (phase !== "entering") {
      return;
    }

    let inner = 0;
    const outer = requestAnimationFrame(() => {
      inner = requestAnimationFrame(() => setPhase("shown"));
    });

    return () => {
      cancelAnimationFrame(outer);
      cancelAnimationFrame(inner);
    };
  }, [phase]);

  async function copyCode(index: number, code: string) {
    await navigator.clipboard.writeText(code);
    setCodeCopied((copied) => ({ ...copied, [index]: true }));
    setTimeout(
      () => setCodeCopied((copied) => ({ ...copied, [index]: false })),
      2000,
    );
  }

  const step = steps[displayStep];

  const handleOpen = () => {
    setCurrentStep(0);
    if (onOpen) {
      onOpen();
    }
  };

  const dialogTitle = title || (step ? step.title : "Steps");

  const handleDone = () => {
    setCurrentStep(0);
    // Trigger close on parent modal container
    const closeButton = contentEl
      ?.closest(".relative")
      ?.querySelector("button") as HTMLButtonElement | null;
    closeButton?.click();
  };

  return (
    <Dialog openButton={openButton} title={dialogTitle} onOpen={handleOpen}>
      {steps.length === 0 ? (
        <div className="py-6 text-center text-gray-500 text-sm">
          No steps required. Your setup is ready.
        </div>
      ) : step ? (
        <>
          {steps.length > 1 && (
            <div className="flex items-center justify-between mb-3 text-xs text-stone-500 font-medium">
              <span>
                Step {displayStep + 1} of {steps.length}
              </span>
              <div className="flex items-center gap-1.5">
                {steps.map((_, i) => (
                  <div
                    key={i}
                    className={`h-1.5 rounded-full transition-all duration-300 ${
                      i === displayStep
                        ? "w-6 bg-odoo"
                        : i < displayStep
                          ? "w-2.5 bg-odoo/40"
                          : "w-2.5 bg-stone-200"
                    }`}
                  />
                ))}
              </div>
            </div>
          )}

          <div
            className="overflow-hidden transition-[height] duration-300 ease-in-out"
            style={{ height: contentHeight }}
          >
            <div ref={setContentEl} className={`${CONTENT_PHASE_CLASS[phase]}`}>
              <h3 className="font-semibold text-stone-900 text-base mb-2">
                {renderFormattedText(step.title)}
              </h3>
              <p className="text-gray-600 text-sm whitespace-pre-line leading-relaxed">
                {renderFormattedText(step.desc)}
              </p>

              {step.link && (
                <a
                  href={step.link}
                  target="_blank"
                  rel="noreferrer"
                  onClick={(event) => {
                    event.preventDefault();
                    BrowserOpenURL(step.link!);
                  }}
                  className="inline-flex items-center mt-3 px-3 py-2 rounded-lg border border-stone-300 text-stone-600 hover:bg-stone-50 hover:border-stone-400 text-sm transition font-medium"
                >
                  {step.linkLabel}
                </a>
              )}

              {step.image && (
                <img
                  src={step.image}
                  alt={step.title}
                  className="max-w-full mt-3 rounded-lg border border-stone-200 shadow-xs"
                />
              )}

              {step.codes?.map((code, index) => (
                <div
                  key={index}
                  className="mt-3.5 rounded-xl bg-slate-900 border border-slate-800 overflow-hidden shadow-inner"
                >
                  <div className="flex items-center justify-between px-3.5 py-1.5 bg-slate-800/80 border-b border-slate-700/60 text-[11px] text-slate-300 font-mono">
                    <span className="flex items-center gap-1.5">
                      <span className="w-2 h-2 rounded-full bg-emerald-400 shrink-0" />
                      Command
                    </span>
                    <button
                      type="button"
                      className="inline-flex items-center gap-1 px-2.5 py-1 rounded-md text-xs font-medium bg-slate-700 text-slate-200 hover:bg-slate-600 hover:text-white transition cursor-pointer shadow-xs"
                      onClick={() => copyCode(index, code)}
                    >
                      {codeCopied[index] ? (
                        <span className="text-emerald-400 font-semibold">
                          ✓ Copied
                        </span>
                      ) : (
                        <span>Copy</span>
                      )}
                    </button>
                  </div>
                  <pre className="p-3.5 text-emerald-400 text-xs sm:text-sm font-mono overflow-x-auto select-all leading-relaxed whitespace-pre">
                    {code}
                  </pre>
                </div>
              ))}
            </div>
          </div>

          <div className="flex gap-2 pt-5">
            {currentStep > 0 && (
              <button
                type="button"
                className="flex-1 py-2 rounded-lg border border-stone-300 text-stone-600 hover:bg-stone-50 hover:border-stone-400 cursor-pointer whitespace-nowrap transition-colors text-sm font-medium"
                onClick={back}
              >
                Back
              </button>
            )}
            {currentStep < steps.length - 1 ? (
              <button
                type="button"
                className="flex-1 py-2 rounded-lg bg-odoo text-white hover:bg-odoo-dark whitespace-nowrap cursor-pointer transition-colors text-sm font-medium shadow-xs"
                onClick={() => next(steps.length)}
              >
                Next
              </button>
            ) : (
              <button
                type="button"
                className="flex-1 py-2 rounded-lg bg-odoo text-white hover:bg-odoo-dark whitespace-nowrap cursor-pointer transition-colors text-sm font-medium shadow-xs"
                onClick={handleDone}
              >
                Done
              </button>
            )}
          </div>
        </>
      ) : null}
    </Dialog>
  );
}

import { Dialog } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import type { ReactNode } from "react";

export function Modal({ open, onOpenChange, title, description, children }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Backdrop className="dialog-backdrop" />
        <Dialog.Viewport className="dialog-viewport">
          <Dialog.Popup className="dialog-popup">
            <header className="dialog-header">
              <div>
                <Dialog.Title className="dialog-title">{title}</Dialog.Title>
                {description ? <Dialog.Description className="dialog-description">{description}</Dialog.Description> : null}
              </div>
              <Dialog.Close className="icon-button" aria-label="关闭"><X size={18} /></Dialog.Close>
            </header>
            {children}
          </Dialog.Popup>
        </Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

import { Dialog } from "@base-ui/react/dialog";
import type { ReactNode } from "react";

export function Drawer({ open, onClose, ariaLabel, children }: {
  open: boolean;
  onClose: () => void;
  ariaLabel: string;
  children: ReactNode;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={(nextOpen) => { if (!nextOpen) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Backdrop className="drawer-backdrop" />
        <Dialog.Viewport className="drawer-viewport">
          <Dialog.Popup className="frequency-drawer" aria-label={ariaLabel} initialFocus>
            {children}
          </Dialog.Popup>
        </Dialog.Viewport>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

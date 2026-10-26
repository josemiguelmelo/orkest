import { useState, type ReactNode } from 'react';
import { AlertCircle, Inbox, LoaderCircle, Plus } from 'lucide-react';
import { Button } from './button';
import { Dialog } from './dialog';

export function PageHeader({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <header className="page-header">
      <div>
        <p className="eyebrow">WORKSPACE MANAGEMENT</p>
        <h1>{title}</h1>
        <p className="muted">{description}</p>
      </div>
      <div className="actions">{children}</div>
    </header>
  );
}
export function Loading() {
  return (
    <div className="empty" role="status">
      <LoaderCircle className="spin" size={24} />
      Loading…
    </div>
  );
}
export function ErrorState({ error, retry }: { error: unknown; retry?: () => void }) {
  return (
    <div className="notice error" role="alert">
      <AlertCircle size={18} />
      <div>
        {error instanceof Error ? error.message : 'Something went wrong.'}
        {retry && (
          <Button variant="outline" size="sm" onClick={retry}>
            Try again
          </Button>
        )}
      </div>
    </div>
  );
}
export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="empty">
      <Inbox size={30} />
      <h3>{title}</h3>
      <div className="muted">{children}</div>
    </div>
  );
}
export function Badge({ children }: { children: ReactNode }) {
  return <span className="badge">{children}</span>;
}

export function CreateButton({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return (
    <Button onClick={onClick}>
      <Plus size={16} />
      {children}
    </Button>
  );
}
export function ConfirmButton({
  label = 'Delete',
  title,
  description,
  onConfirm,
  pending = false,
}: {
  label?: string;
  title: string;
  description: string;
  onConfirm: () => Promise<unknown>;
  pending?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  async function confirm() {
    setBusy(true);
    setError(undefined);
    try {
      await onConfirm();
      setOpen(false);
    } catch (reason) {
      setError(reason);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Button variant="outline" size="sm" disabled={pending} onClick={() => setOpen(true)}>
        {label}
      </Button>
      <Dialog
        open={open}
        onOpenChange={(value) => {
          if (!busy) {
            setOpen(value);
            setError(undefined);
          }
        }}
        title={title}
        description={description}
      >
        {error !== undefined && <ErrorState error={error} />}
        <div className="form-actions">
          <Button variant="outline" disabled={busy} onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={busy}
            onClick={() => {
              void confirm();
            }}
          >
            {busy ? 'Working…' : label}
          </Button>
        </div>
      </Dialog>
    </>
  );
}
export function date(value: string) {
  return new Date(value).toLocaleString();
}
